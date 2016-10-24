package uiway

import "sync"

type Channel struct {
	sync.RWMutex
	sessions map[uint32]*Session
	State    interface{}
}

func NewChannel() *Channel {
	return &Channel{
		sessions: make(map[uint32]*Session),
	}
}

func (c *Channel) len() int {
	c.RLock()
	defer c.RUnlock()
	return len(c.sessions)
}

func (c *Channel) fetch(cb func(*Session)) {
	c.RLock()
	defer c.RUnlock()
	for _, sess := range c.sessions {
		cb(sess)
	}
}

func (c *Channel) get(key uint32) *Session {
	c.RLock()
	defer c.RUnlock()
	sess, _ := c.sessions[key]
	return sess
}

func (c *Channel) put(key uint32, session *Session) {
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

func (c *Channel) remove(key uint32, session *Session) {
	session.removeCloseCallback(c)
	delete(c.sessions, key)
}

func (c *Channel) removeByKey(key uint32) bool {
	c.Lock()
	defer c.Unlock()
	sess, exists := c.sessions[key]
	if exists {
		c.remove(key, sess)
	}
	return exists
}

func (c *Channel) Close() {
	c.Lock()
	defer c.Unlock()
	for key, sess := range c.sessions {
		c.remove(key, sess)
	}
}
