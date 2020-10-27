package http_capture

import (
	"net/http"
	"sync"
)

type reqWrapper struct {
	req      *http.Request
	waitChan chan byte // 用于请求的等待
}
type requestList struct {
	sync.Mutex
	id2req map[int32]reqWrapper
	nextId int32
}

func newRequestList() *requestList {
	return &requestList{
		Mutex:  sync.Mutex{},
		id2req: make(map[int32]reqWrapper),
		nextId: 0,
	}
}

func (l *requestList) getReqWrapper(id int32) reqWrapper {
	l.Lock()
	defer l.Unlock()
	return l.id2req[id]
}

func (l *requestList) getReq(id int32) *http.Request {
	return l.getReqWrapper(id).req
}

func (l *requestList) putReq(req *http.Request) (id int32) {
	l.Lock()
	defer l.Unlock()
	id = l.nextId
	l.nextId++
	l.id2req[id] = reqWrapper{req: req, waitChan: make(chan byte)}
	return id
}

func (l *requestList) wait(id int32) {
	<-l.getReqWrapper(id).waitChan
}

func (l *requestList) remove(id int32) {
	l.Lock()
	defer l.Unlock()
	delete(l.id2req, id)
}

func (l *requestList) size() int32 {
	l.Lock()
	defer l.Unlock()
	return int32(len(l.id2req))
}
