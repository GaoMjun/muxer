package muxer

import (
	"bytes"
	"encoding/binary"
	"io"
	"sync"
)

type Stream struct {
	muxer *Muxer
	ID    int
	cmd   int

	dataBuffer *buffer

	closed    bool
	locker    *sync.RWMutex
	sendClose bool

	firstFrame bool
}

func newStream(id int, muxer *Muxer) (stream *Stream) {
	stream = &Stream{}
	stream.ID = id
	stream.muxer = muxer
	stream.dataBuffer = newBuffer()
	stream.locker = &sync.RWMutex{}
	stream.firstFrame = true
	return
}

func (self *Stream) Feed(p []byte) {
	self.dataBuffer.write(p)
}

func (self *Stream) Read(p []byte) (n int, err error) {
	n, err = self.dataBuffer.Read(p)
	return
}

func (self *Stream) Write(p []byte) (n int, err error) {
	self.locker.RLock()
	closed := self.closed
	self.locker.RUnlock()

	if closed {
		err = io.ErrClosedPipe
		return
	}

	offset := 0
	for offset < len(p) {
		length := len(p) - offset

		if length > int(^uint16(0)) {
			length = int(^uint16(0))
		}

		if self.firstFrame {
			self.firstFrame = false
			self.cmd = CMD_OPENSTREAM
		} else {
			self.cmd = CMD_DATASTREAM
		}
		if _, err = self.writeFrame(p[offset : offset+length]); err != nil {
			return
		}

		offset += length
	}

	n = len(p)
	return
}

func (self *Stream) Close() (err error) {
	self.locker.Lock()
	if self.closed {
		self.locker.Unlock()
		return
	}
	self.closed = true
	self.locker.Unlock()

	if self.sendClose {
		self.cmd = CMD_CLOSESTREAM
		if _, err = self.writeFrame(nil); err != nil {
			return
		}
	}

	self.muxer.delStream(self.ID)

	self.dataBuffer.eof()
	return
}

func (self *Stream) writeFrame(p []byte) (n int, err error) {
	buffer := &bytes.Buffer{}

	// write signature
	if err = binary.Write(buffer, binary.LittleEndian, []byte("mux")); err != nil {
		return
	}

	// write stream id
	id := make([]byte, 2)
	id[0] = byte(self.ID >> 8 & 0xFF)
	id[1] = byte(self.ID >> 0 & 0xFF)
	if err = binary.Write(buffer, binary.LittleEndian, id); err != nil {
		return
	}

	// write cmd
	cmd := make([]byte, 1)
	cmd[0] = byte(self.cmd)
	if err = binary.Write(buffer, binary.LittleEndian, cmd); err != nil {
		return
	}

	if len(p) > 0 {
		// write data length
		length := make([]byte, 2)
		length[0] = byte(len(p) >> 8 & 0xFF)
		length[1] = byte(len(p) >> 0 & 0xFF)
		if err = binary.Write(buffer, binary.LittleEndian, length); err != nil {
			return
		}

		// write data
		if err = binary.Write(buffer, binary.LittleEndian, p); err != nil {
			return
		}
	}

	self.muxer.dataOutCh <- buffer.Bytes()

	n = len(p)
	return
}
