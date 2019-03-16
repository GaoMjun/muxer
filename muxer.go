package muxer

import (
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
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
}

func New(conn io.ReadWriter) (muxer *Muxer) {
	muxer = &Muxer{}
	muxer.conn = conn
	muxer.idsinused = map[int]int{}
	muxer.idsnoused = map[int]int{}
	muxer.streams = map[int]*Stream{}
	muxer.locker = &sync.RWMutex{}
	muxer.dataOutCh = make(chan []byte)
	muxer.streamCh = make(chan *Stream)

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

	self.streams[id] = stream
	return
}

func (self *Muxer) AcceptStream() (stream *Stream) {
	stream = <-self.streamCh
	return
}

func (self *Muxer) writeLoop() {
	for data := range self.dataOutCh {
		if _, err := self.conn.Write(data); err != nil {
			log.Println(err)
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
		if err != nil {
			log.Println(err)
		}
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
		case 0: // open stream
			if stream == nil {
				if stream, err = self.OpenStream(); err != nil {
					log.Println(err)
				}

				self.streamCh <- stream
			}
		case 1: // close stream
			if stream != nil {
				close(stream.dataInCh)
				stream.closed = true
				self.delStream(stream.ID)
			}
			continue
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

		if stream != nil && length > 0 {
			// read data
			buffer = make([]byte, length)
			if _, err = io.ReadFull(self.conn, buffer); err != nil {
				return
			}

			stream.dataInCh <- buffer
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
