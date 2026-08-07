// SPDX-License-Identifier: Apache-2.0

package publicapi

import (
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
)

// staleCache retains the last good encoded response per request key and the
// last good proto message per Connect method. It backs the availability
// promise: a public read NEVER returns 500 — if the database is down we
// serve the newest thing we ever produced, honestly labeled with
// X-Data-Stale: true (and, on a cold start, an empty-but-valid payload).
type staleCache struct {
	mu     sync.RWMutex
	bodies map[string]cachedBody
	protos map[string]cachedProto
}

type cachedBody struct {
	body        []byte
	contentType string
	storedAt    time.Time
}

type cachedProto struct {
	msg      proto.Message
	storedAt time.Time
}

func newStaleCache() *staleCache {
	return &staleCache{bodies: map[string]cachedBody{}, protos: map[string]cachedProto{}}
}

func (c *staleCache) storeBody(key string, body []byte, contentType string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bodies[key] = cachedBody{body: body, contentType: contentType, storedAt: time.Now()}
}

func (c *staleCache) getBody(key string) (cachedBody, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	b, ok := c.bodies[key]
	return b, ok
}

func (c *staleCache) storeProto(key string, msg proto.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.protos[key] = cachedProto{msg: proto.Clone(msg), storedAt: time.Now()}
}

func (c *staleCache) getProto(key string) (proto.Message, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.protos[key]
	if !ok {
		return nil, false
	}
	return proto.Clone(p.msg), true
}
