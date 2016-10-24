package uniway

import (
	"container/list"
	"errors"
	"sync"
	"sync/atomic"
)

var (
	ErrSessionClosed  = errors.New("session Closed")
	ErrSessionBlocked = errors.New("session Blocked")
)

var globalSessionID uint64

type Session struct {
	id        uint64
	codec     Codec
	manager   *sessionManager
	sendChan  chan interface{}
	sendMutex sync.RWMutex

	closeFlag      int32
	closeChan      chan struct{}
	closeMutex     sync.Mutex
	closeCallbacks *list.List

	State interface{}
}

func newSession(codec Codec, sendChanSize int) *Session {
	return newSessionWithManager(nil, codec, sendChanSize)
}

func newSessionWithManager(manager *sessionManager, codec Codec, sendChanSize int) *Session {
	sess := &Session{
		codec:     codec,
		manager:   manager,
		closeChan: make(chan struct{}),
		id:        atomic.AddUint64(&globalSessionID, 1),
	}
	if sendChanSize > 0 {
		sess.sendChan = make(chan interface{}, sendChanSize)
		go sess.sendLoop()
	}
	return sess
}

func (s *Session) IsClosed() bool {
	return atomic.LoadInt32(&s.closeFlag) == 1
}

func (s *Session) Close() error {
	if atomic.CompareAndSwapInt32(&s.closeFlag, 0, 1) {
		close(s.closeChan)

		if s.sendChan != nil {
			s.sendMutex.Lock()
			close(s.sendChan)
			if clear, ok := s.codec.(ClearSendChan); ok {
				clear.ClearSendChan(s.sendChan)
			}
			s.sendMutex.Unlock()
		}

		err := s.codec.Close()
		if s.manager != nil {
			s.manager.delSession(s)
		}
		s.invokeCloseCallbacks()
		return err
	}
	return ErrSessionClosed
}

func (s *Session) getCodec() Codec {
	return s.codec
}

func (s *Session) Receive() (interface{}, error) {
	msg, err := s.codec.Receive()
	if err != nil {
		s.Close()
	}
	return msg, err
}

func (s *Session) sendLoop() {
	defer s.Close()
	for {
		select {
		case msg, ok := <-s.sendChan:
			if !ok || s.codec.Send(msg) != nil {
				return
			}
		case <-s.closeChan:
			return
		}
	}
}

func (s *Session) Send(msg interface{}) error {
	if s.IsClosed() {
		return ErrSessionClosed
	}

	if s.sendChan == nil {
		err := s.codec.Send(msg)
		if err != nil {
			s.Close()
		}
		return err
	}

	s.sendMutex.RLock()
	select {
	case s.sendChan <- msg:
		s.sendMutex.RUnlock()
		return nil
	case <-s.closeChan:
		s.sendMutex.RUnlock()
		return ErrSessionClosed
	default:
		s.sendMutex.RUnlock()
		s.Close()
		return ErrSessionClosed
	}
}

type closeCallback struct {
	Handler interface{}
	Func    func()
}

func (s *Session) addCloseCallback(handler interface{}, callback func()) {
	if s.IsClosed() {
		return
	}

	s.closeMutex.Lock()
	defer s.closeMutex.Unlock()

	if s.closeCallbacks == nil {
		s.closeCallbacks = list.New()
	}

	s.closeCallbacks.PushBack(closeCallback{handler, callback})
}

func (s *Session) removeCloseCallback(handler interface{}) {
	if s.IsClosed() {
		return
	}

	s.closeMutex.Lock()
	defer s.closeMutex.Unlock()

	for i := s.closeCallbacks.Front(); i != nil; i = i.Next() {
		if i.Value.(closeCallback).Handler == handler {
			s.closeCallbacks.Remove(i)
			return
		}
	}
}

func (s *Session) invokeCloseCallbacks() {
	s.closeMutex.Lock()
	defer s.closeMutex.Unlock()

	if s.closeCallbacks == nil {
		return
	}

	for i := s.closeCallbacks.Front(); i != nil; i = i.Next() {
		cb := i.Value.(closeCallback)
		cb.Func()
	}
}

//==============================

const sessionMapNum = 32

type sessionManager struct {
	sessionMaps [sessionMapNum]sessionMap
	disposeFlag bool
	disposeOnce sync.Once
	disposeWait sync.WaitGroup
}

type sessionMap struct {
	sync.RWMutex
	sessions map[uint64]*Session
}

func newSessionManager() *sessionManager {
	manager := &sessionManager{}
	for i := 0; i < len(manager.sessionMaps); i++ {
		manager.sessionMaps[i].sessions = make(map[uint64]*Session)
	}
	return manager
}

func (sm *sessionManager) dispose() {
	sm.disposeOnce.Do(func() {
		sm.disposeFlag = true
		for i := 0; i < sessionMapNum; i++ {
			smap := &sm.sessionMaps[i]
			smap.Lock()
			for _, sess := range smap.sessions {
				sess.Close()
			}
			smap.Unlock()
		}
		sm.disposeWait.Wait()
	})
}

func (sm *sessionManager) newSession(codec Codec, sendChanSize int) *Session {
	sess := newSessionWithManager(sm, codec, sendChanSize)
	sm.putSession(sess)
	return sess
}

func (sm *sessionManager) getSession(sessID uint64) *Session {
	smap := &sm.sessionMaps[sessID%sessionMapNum]
	smap.RLock()
	defer smap.Unlock()

	sess, _ := smap.sessions[sessID]
	return sess
}

func (sm *sessionManager) putSession(sess *Session) {
	smap := &sm.sessionMaps[sess.id%sessionMapNum]
	smap.Lock()
	defer smap.Unlock()

	smap.sessions[sess.id] = sess
	sm.disposeWait.Add(1)
}

func (sm *sessionManager) delSession(sess *Session) {
	if sm.disposeFlag {
		sm.disposeWait.Done()
		return
	}

	smap := &sm.sessionMaps[sess.id%sessionMapNum]
	smap.Lock()
	defer smap.Unlock()

	delete(smap.sessions, sess.id)
	sm.disposeWait.Done()
}
