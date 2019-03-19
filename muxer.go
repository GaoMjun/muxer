package muxer

import (
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
)

const (
	CMD_OPENSTREAM  int = 0
	CMD_CLOSESTREAM int = 1
	CMD_DATASTREAM      = 2
)

type Muxer struct {
	conn io.ReadWriter

	id        int
	idsinused map[int]int
	idsnoused map[int]int
	streams   map[int]*Stream
	locker    *sync.RWMutex

	dataOutCh chan []byte

	streamCh chan *Stream

	end chan error
}

func New(conn io.ReadWriter) (muxer *Muxer) {
	muxer = &Muxer{}
	muxer.conn = conn
	muxer.idsinused = map[int]int{}
	muxer.idsnoused = map[int]int{}
	muxer.streams = map[int]*Stream{}
	muxer.locker = &sync.RWMutex{}
	muxer.dataOutCh = make(chan []byte, 32)
	muxer.streamCh = make(chan *Stream)
	muxer.end = make(chan error)

	go muxer.readLoop()
	go muxer.writeLoop()
	return
}

func (self *Muxer) OpenStream() (stream *Stream, err error) {
	self.locker.Lock()
	defer self.locker.Unlock()

	var (
		id int
	)
	if id, err = self.getID(); err != nil {
		return
	}

	stream = newStream(id, self)
	stream.sendClose = true

	self.streams[id] = stream
	return
}

func (self *Muxer) AcceptStream() (stream *Stream, err error) {
	select {
	case err = <-self.end:
	case stream = <-self.streamCh:
		stream.sendClose = false
	}

	return
}

func (self *Muxer) Wait() {
	<-self.end
}

func (self *Muxer) writeLoop() {
	var (
		err error
	)
	defer func() {
		self.end <- err
	}()

	for data := range self.dataOutCh {
		if _, err := self.conn.Write(data); err != nil {
			return
		}
	}
}

func (self *Muxer) readLoop() {
	var (
		err      error
		cmd      int
		streamID int
		length   int
		stream   *Stream
	)
	defer func() {
		self.end <- err
	}()

	for {
		// read signature
		buffer := make([]byte, 3)
		if _, err = io.ReadFull(self.conn, buffer); err != nil {
			return
		}
		if string(buffer) != "mux" {
			err = errors.New("parsing signature failed")
			return
		}

		// read stream id
		buffer = make([]byte, 2)
		if _, err = io.ReadFull(self.conn, buffer); err != nil {
			return
		}
		streamID = int(buffer[0])<<8 | int(buffer[1])

		stream, _ = self.getStream(streamID)

		// read cmd
		buffer = make([]byte, 1)
		if _, err = io.ReadFull(self.conn, buffer); err != nil {
			return
		}
		cmd = int(buffer[0])

		switch cmd {
		case CMD_OPENSTREAM: // open stream
			if stream == nil {
				if stream, err = self.OpenStream(); err != nil {
					log.Println(err)
					break
				}

				go func() {
					self.streamCh <- stream
				}()

			}
		case CMD_CLOSESTREAM: // close stream
			if stream != nil {
				stream.Close()
			}
			continue
		case CMD_DATASTREAM:

		default:
			err = errors.New("parsing cmd failed")
			return
		}

		// read data length
		buffer = make([]byte, 2)
		if _, err = io.ReadFull(self.conn, buffer); err != nil {
			return
		}
		length = int(buffer[0])<<8 | int(buffer[1])
		log.Println("readLoop", cmd, length)

		if length > 0 {
			// read data
			buffer = make([]byte, length)
			if _, err = io.ReadFull(self.conn, buffer); err != nil {
				return
			}

			if stream != nil {
				stream.Feed(buffer)
			}
		}
	}
}

func (self *Muxer) getStream(id int) (stream *Stream, err error) {
	self.locker.RLock()
	defer self.locker.RUnlock()

	if _, ok := self.streams[id]; !ok {
		err = errors.New(fmt.Sprint("no stream ", id))
		return
	}

	stream = self.streams[id]
	return
}

func (self *Muxer) delStream(id int) {
	self.locker.Lock()
	defer self.locker.Unlock()

	if _, ok := self.streams[id]; ok {
		delete(self.streams, id)
		self.delID(id)
	}
}

func (self *Muxer) getID() (id int, err error) {
	if len(self.idsnoused) > 0 {
		for k, v := range self.idsnoused {
			delete(self.idsnoused, k)
			self.idsinused[k] = v
			id = v
			return
		}
		return
	}

	if self.id > int(^uint16(0)) {
		err = errors.New("no available id")
		return
	}

	id = self.id
	self.id++

	self.idsinused[id] = id
	return
}

func (self *Muxer) delID(id int) {
	if _, ok := self.idsinused[id]; ok {
		delete(self.idsinused, id)
		self.idsnoused[id] = id
	}
}
