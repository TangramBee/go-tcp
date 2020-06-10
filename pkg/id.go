package pkg

import "sync"

var (
	SessionId int64
	SessionIdCh = make(chan string, 100)
	Sessions * Session
)

type Session struct {
	SessionId string
	Manager *sync.Map
	Route *sync.Map
}

type Message struct {
	Route string `json:"route"`
	Id    int64		  `json:"id"`
	Method string	  `json:"method"`
	Data   []byte     `json:"data"`
	Heartbeat string   `json:"heartbeat"`
}
