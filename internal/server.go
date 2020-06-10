package internal

import (
	"encoding/binary"
	"encoding/json"
	"go.uber.org/zap"
	"io"
	"net"
	"sync"
	"time"

	"go-web/pkg/logger"
)

type GoWeb interface {
	Do(c *Conn, message *pkg.Message)
}

type Conn struct {
	c net.Conn
	Id string
	m  *pkg.Session
	fn       GoWeb
	close     chan bool
	ReadConn chan []byte
	Response chan []byte
	sendTime time.Time
	retries   int
	Lock     *sync.RWMutex
	Once     *sync.Once
}

func NewConn(conn net.Conn, id string) *Conn {
	return &Conn{
		c:        conn,
		Id:       id,
		m:        pkg.Sessions,
		Lock:     &sync.RWMutex{},
		sendTime: time.Now(),
		close:     make(chan bool),
		ReadConn: make(chan []byte, 1000),
		Response: make(chan []byte, 1000),
		Once:     &sync.Once{},
	}
}

func (c *Conn) Start() {
	go pkg.Wrapper(c.read)
	go pkg.Wrapper(c.do)
	go pkg.Wrapper(c.write)
	go pkg.Wrapper(c.synSend)
}

// 接收连接的消息
func (c *Conn) read() {
	// 读头部
	sizeData := make([]byte, 2)
	for {
		select {
		case <-c.close:
			return
		default:
			if _, err := io.ReadFull(c.c, sizeData); err != nil {
				logger.Logger.Warn("read conn header err",  zap.Error(err))
				c.Close()
				return
			}

			data := make([]byte, uint16(binary.BigEndian.Uint16(sizeData)))
			if _, err := io.ReadFull(c.c, data); err != nil {
				logger.Logger.Warn("read conn body err", zap.Error(err))
				c.Close()
				return
			}
			// 通过chan传输数据，加快并发处理效率
			c.ReadConn <- data

		}
	}
}

// 处理接收的消息
func (c *Conn) do() {
	for data := range c.ReadConn {
		message := &pkg.Message {
			// 内存页的大小是1024的整数倍，slice每次扩容2倍
			Data: make([]byte, 0, 1024),
		}
		err := json.Unmarshal(data, message)
		if err != nil {
			logger.Logger.Warn("unmarshal data err", zap.Error(err))
			continue
		}

		// 处理消息
		go c.fn.Do(c, message)
	}
}

// 发送消息给连接
func (c *Conn) Write(data []byte) {
	c.Response <- data
}

// 打包发送消息
func (c *Conn) write() {
	for data := range c.Response {
		result := make([]byte, 2)
		// 头部存储数据长度
		binary.BigEndian.PutUint16(result, uint16(len(data)))
		// 数据拼接到切片中，发送
		result = append(result, data...)
		if _, err := c.c.Write(result); err != nil {
			c.m.Manager.Delete(c.Id)
			c.Close()
		}
	}
}

func (c *Conn) Close() {
	c.Once.Do(func() {
		c.m.Manager.Delete(c.Id)
		close(c.close)
		close(c.Response)
		close(c.ReadConn)
		c.c.Close()
	})
}

func CreateConn(conn net.Conn, id string, f Woodpeck) {
	defer func() {
		if err := recover(); err != nil {
			logger.Error(err)
		}
	}()
	c := NewConn(conn, id)
	c.fn = f
	pkg.Sessions.Manager.Set(id, c)
	c.Start()
}