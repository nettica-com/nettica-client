package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

const maxPeersPerRoom = 5
const conferenceAddr = "0.0.0.0:3001"

// PeerInfo is the lightweight peer descriptor used in room_state and peer_joined.
type PeerInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar,omitempty"`
}

// SignalMessage is the envelope for all WebRTC signaling messages.
//
// Client → Server:
//
//	"join"        { roomId, name, avatar }
//	"offer"       { to, sdp }
//	"answer"      { to, sdp }
//	"candidate"   { to, candidate, sdpMid, sdpMLineIndex }
//	"media_state" { audioMuted, videoOff }
//	"leave"
//
// Server → Client:
//
//	"welcome"     { id }
//	"room_state"  { peers:[{id,name,avatar}] }
//	"peer_joined" { id, name, avatar }
//	"peer_left"   { id }
//	"offer"       { from, sdp }
//	"answer"      { from, sdp }
//	"candidate"   { from, candidate, sdpMid, sdpMLineIndex }
//	"media_state" { from, audioMuted, videoOff }
//	"error"       { message }
type SignalMessage struct {
	Type   string `json:"type"`
	// Identity / room
	ID     string `json:"id,omitempty"`
	RoomID string `json:"roomId,omitempty"`
	Name   string `json:"name,omitempty"`
	Avatar string `json:"avatar,omitempty"`
	// Room state
	Peers []PeerInfo `json:"peers,omitempty"`
	// Relay routing
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	// SDP (offer / answer)
	SDP string `json:"sdp,omitempty"`
	// ICE candidate
	Candidate     string  `json:"candidate,omitempty"`
	SdpMid        *string `json:"sdpMid,omitempty"`
	SdpMLineIndex *int    `json:"sdpMLineIndex,omitempty"`
	// Media state (mute / video-off)
	AudioMuted *bool `json:"audioMuted,omitempty"`
	VideoOff   *bool `json:"videoOff,omitempty"`
	// Error
	Message string `json:"message,omitempty"`
}

// peer represents a single connected WebSocket client.
type peer struct {
	id     string
	name   string
	avatar string
	conn   *websocket.Conn
	send   chan []byte
	mu     sync.Mutex // guards conn writes
}

// room holds all peers currently in a conference room.
type room struct {
	id    string
	peers map[string]*peer
	mu    sync.RWMutex
}

func (r *room) add(p *peer) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.peers) >= maxPeersPerRoom {
		return false
	}
	r.peers[p.id] = p
	return true
}

func (r *room) remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.peers, id)
}

// peerInfos returns PeerInfo for all peers except the one to exclude.
func (r *room) peerInfos(exclude string) []PeerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	infos := make([]PeerInfo, 0, len(r.peers))
	for id, p := range r.peers {
		if id != exclude {
			infos = append(infos, PeerInfo{ID: p.id, Name: p.name, Avatar: p.avatar})
		}
	}
	return infos
}

func (r *room) size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.peers)
}

// broadcast enqueues data for every peer except excludeID.
func (r *room) broadcast(data []byte, excludeID string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for id, p := range r.peers {
		if id == excludeID {
			continue
		}
		select {
		case p.send <- data:
		default:
			log.Warnf("conference: peer %s send buffer full, dropping broadcast", id)
		}
	}
}

// relay enqueues data for a single target peer.
func (r *room) relay(toID string, data []byte) {
	r.mu.RLock()
	p, ok := r.peers[toID]
	r.mu.RUnlock()
	if !ok {
		log.Warnf("conference: relay target %s not found in room %s", toID, r.id)
		return
	}
	select {
	case p.send <- data:
	default:
		log.Warnf("conference: peer %s send buffer full, dropping relay", toID)
	}
}

// roomManager holds all active rooms.
type roomManager struct {
	rooms map[string]*room
	mu    sync.RWMutex
}

var conference = &roomManager{rooms: make(map[string]*room)}

func (rm *roomManager) getOrCreate(id string) *room {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if r, ok := rm.rooms[id]; ok {
		return r
	}
	r := &room{id: id, peers: make(map[string]*peer)}
	rm.rooms[id] = r
	return r
}

func (rm *roomManager) cleanup(id string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if r, ok := rm.rooms[id]; ok {
		r.mu.RLock()
		empty := len(r.peers) == 0
		r.mu.RUnlock()
		if empty {
			delete(rm.rooms, id)
			log.Infof("conference: room %s closed", id)
		}
	}
}

// wsUpgrader accepts connections from any origin (mobile apps, LAN, VPN).
var wsUpgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}

func generatePeerID() string {
	b := make([]byte, 8)
	rand.Read(b) //nolint:errcheck
	return hex.EncodeToString(b)
}

func sendJSON(p *peer, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Errorf("conference: marshal error: %v", err)
		return
	}
	select {
	case p.send <- data:
	default:
		log.Warnf("conference: peer %s send buffer full at sendJSON", p.id)
	}
}

// writePump drains the peer's send channel and writes to the WebSocket.
// A ping is sent every 30 s so the mobile client knows the connection is alive.
func (p *peer) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		p.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-p.send:
			p.mu.Lock()
			p.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				p.conn.WriteMessage(websocket.CloseMessage, []byte{}) //nolint:errcheck
				p.mu.Unlock()
				return
			}
			err := p.conn.WriteMessage(websocket.TextMessage, msg)
			p.mu.Unlock()
			if err != nil {
				log.Debugf("conference: write error for peer %s: %v", p.id, err)
				return
			}

		case <-ticker.C:
			p.mu.Lock()
			p.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			err := p.conn.WriteMessage(websocket.PingMessage, nil)
			p.mu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

// conferenceHandler handles WebSocket connections at /.
// The client sends a "join" message first to specify the room and identity.
func conferenceHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("conference: WebSocket upgrade failed: %v", err)
		return
	}

	p := &peer{
		id:   generatePeerID(),
		conn: conn,
		send: make(chan []byte, 64),
	}

	go p.writePump()

	// Wait for the "join" message to learn which room and who the peer is.
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		log.Debugf("conference: no join message received: %v", err)
		close(p.send)
		return
	}

	var join SignalMessage
	if err := json.Unmarshal(data, &join); err != nil || join.Type != "join" || join.RoomID == "" {
		sendJSON(p, SignalMessage{Type: "error", Message: "first message must be {type:join, roomId:…}"})
		conn.Close()
		close(p.send)
		return
	}

	roomID := Sanitize(join.RoomID)
	if roomID == "" {
		sendJSON(p, SignalMessage{Type: "error", Message: "invalid room ID"})
		conn.Close()
		close(p.send)
		return
	}

	p.name = join.Name
	p.avatar = join.Avatar

	rm := conference.getOrCreate(roomID)

	if !rm.add(p) {
		sendJSON(p, SignalMessage{Type: "error", Message: "room is full (maximum 5 users)"})
		// Flush the error before closing.
		if msg, ok := <-p.send; ok {
			conn.WriteMessage(websocket.TextMessage, msg) //nolint:errcheck
		}
		conn.Close()
		close(p.send)
		log.Infof("conference: peer rejected – room %s is full", roomID)
		return
	}

	log.Infof("conference: peer %s (%s) joined room %s (%d/%d)",
		p.id, p.name, roomID, rm.size(), maxPeersPerRoom)

	// Tell the new peer its assigned ID.
	sendJSON(p, SignalMessage{Type: "welcome", ID: p.id})

	// Send the current room roster so the client can initiate offers.
	infos := rm.peerInfos(p.id)
	if infos == nil {
		infos = []PeerInfo{}
	}
	sendJSON(p, SignalMessage{Type: "room_state", Peers: infos})

	// Tell everyone else that a new peer has arrived.
	joined, _ := json.Marshal(SignalMessage{
		Type:   "peer_joined",
		ID:     p.id,
		Name:   p.name,
		Avatar: p.avatar,
	})
	rm.broadcast(joined, p.id)

	// Read pump – runs on this goroutine until the connection closes or "leave".
	defer func() {
		rm.remove(p.id)
		left, _ := json.Marshal(SignalMessage{Type: "peer_left", ID: p.id})
		rm.broadcast(left, p.id)
		close(p.send)
		conn.Close()
		conference.cleanup(roomID)
		log.Infof("conference: peer %s left room %s", p.id, roomID)
	}()

	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Debugf("conference: peer %s unexpected close: %v", p.id, err)
			}
			break
		}
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		var msg SignalMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Warnf("conference: invalid JSON from peer %s: %v", p.id, err)
			continue
		}

		// Always stamp the sender's ID.
		msg.From = p.id

		switch msg.Type {
		case "offer", "answer":
			if msg.To == "" {
				log.Warnf("conference: peer %s sent %q without a 'to' field", p.id, msg.Type)
				continue
			}
			out, _ := json.Marshal(msg)
			rm.relay(msg.To, out)

		case "candidate":
			if msg.To == "" {
				log.Warnf("conference: peer %s sent candidate without a 'to' field", p.id)
				continue
			}
			out, _ := json.Marshal(msg)
			rm.relay(msg.To, out)

		case "media_state":
			out, _ := json.Marshal(msg)
			rm.broadcast(out, p.id)

		case "leave":
			return

		default:
			log.Warnf("conference: unknown message type %q from peer %s", msg.Type, p.id)
		}
	}
}

// startConferenceServer starts the WebSocket signaling server on port 3000.
// It is intentionally separate from the main HTTP API server (127.0.0.1:53280)
// so that mobile clients on the VPN can reach it without the localhost restriction.
func startConferenceServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", conferenceHandler)

	log.Infof("Conference: WebSocket signaling server listening on %s", conferenceAddr)
	if err := http.ListenAndServe(conferenceAddr, mux); err != nil {
		log.Errorf("Conference: server error: %v", err)
	}
}
