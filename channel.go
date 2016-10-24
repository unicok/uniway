package main

import "sync"

type channel struct {
	sync.RWMutex
	sessions map[uint32]*session
	State    interface{}
}

func newChannel() *channel {
	return &channel{
		sessions: make(map[uint32]*session),
	}
}

func (c *channel) len() int {
	c.RLock()
	defer c.RUnlock()
	return len(c.sessions)
}

func (c *channel) fetch(cb func(*session)) {
	c.RLock()
	defer c.RUnlock()
	for _, sess := range c.sessions {
		cb(sess)
	}
}

func (c *channel) get(key uint32) *session {
	c.RLock()
	defer c.RUnlock()
	sess, _ := c.sessions[key]
	return sess
}

func (c *channel) put(key uint32, session *session) {
	c.RLock()
	defer c.RUnlock()
	if sess, exists := c.sessions[key]; exists {
		c.remove(key, sess)
	}
	session.addCloseCallback(c, func() {
		c.removeByKey(key)
	})
	c.sessions[key] = session
}

func (c *channel) remove(key uint32, session *session) {
	session.removeCloseCallback(c)
	delete(c.sessions, key)
}

func (c *channel) removeByKey(key uint32) bool {
	c.Lock()
	defer c.Unlock()
	sess, exists := c.sessions[key]
	if exists {
		c.remove(key, sess)
	}
	return exists
}

func (c *channel) close() {
	c.Lock()
	defer c.Unlock()
	for key, sess := range c.sessions {
		c.remove(key, sess)
	}
}
