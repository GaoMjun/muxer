package muxer

import (
	"bytes"
	"encoding/binary"
	"io"
)

type Stream struct {
	muxer *Muxer
	ID    int
	cmd   int

	dataInCh chan []byte
	buffer   []byte

	closed bool
}

func newStream(id int, muxer *Muxer) (stream *Stream) {
	stream = &Stream{}
	stream.ID = id
	stream.muxer = muxer
	stream.dataInCh = make(chan []byte)
	return
}

func (self *Stream) Read(p []byte) (n int, err error) {
	if len(self.buffer) > 0 {
		if len(p) >= len(self.buffer) {
			n = copy(p, self.buffer)
			self.buffer = nil
			return
		}

		n = copy(p, self.buffer)
		self.buffer = self.buffer[n:]
		return
	}

	var (
		data []byte
		ok   bool
	)
	if data, ok = <-self.dataInCh; !ok {
		self.closed = true
		err = io.ErrUnexpectedEOF
		return
	}

	if len(p) >= len(data) {
		n = copy(p, data)
		return
	}

	n = copy(p, data)
	self.buffer = data[n:]
	return
}

func (self *Stream) Write(p []byte) (n int, err error) {
	if self.closed {
		err = io.ErrClosedPipe
		return
	}

	offset := 0
	for offset < len(p) {
		length := len(p) - offset

		if length > int(^uint16(0)) {
			length = int(^uint16(0))
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
	if self.closed {
		return
	}

	close(self.dataInCh)
	self.closed = true
	self.cmd = 1
	if _, err = self.writeFrame(nil); err != nil {
		return
	}

	self.muxer.delStream(self.ID)
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
