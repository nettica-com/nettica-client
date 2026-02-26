package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

const maxPeersPerRoom = 5
const conferenceAddr = "0.0.0.0:3000"

// SignalMessage is the envelope used for all WebRTC signaling messages.
//
// Client → Server message types:
//
//	"offer"         { to, payload }  – relay SDP offer to a specific peer
//	"answer"        { to, payload }  – relay SDP answer to a specific peer
//	"ice-candidate" { to, payload }  – relay ICE candidate to a specific peer
//
// Server → Client message types:
//
//	"joined"        { peerId, existingPeers }  – sent to the joining peer
//	"peer-joined"   { from }                   – broadcast when a new peer arrives
//	"peer-left"     { from }                   – broadcast when a peer disconnects
//	"offer"         { from, payload }           – relayed SDP offer
//	"answer"        { from, payload }           – relayed SDP answer
//	"ice-candidate" { from, payload }           – relayed ICE candidate
//	"error"         { message }                 – e.g. room full
type SignalMessage struct {
	Type          string          `json:"type"`
	PeerID        string          `json:"peerId,omitempty"`
	ExistingPeers []string        `json:"existingPeers,omitempty"`
	From          string          `json:"from,omitempty"`
	To            string          `json:"to,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	Message       string          `json:"message,omitempty"`
}

// peer represents a single connected WebSocket client.
type peer struct {
	id   string
	conn *websocket.Conn
	send chan []byte
	mu   sync.Mutex // guards conn writes
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

// peerIDs returns the IDs of all peers except the one to exclude.
func (r *room) peerIDs(exclude string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.peers))
	for id := range r.peers {
		if id != exclude {
			ids = append(ids, id)
		}
	}
	return ids
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

// conferenceHandler handles WebSocket connections at /room/{roomId}.
func conferenceHandler(w http.ResponseWriter, r *http.Request) {
	// Parse roomId from path: /room/{roomId}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
	if len(parts) != 2 || parts[0] != "room" || parts[1] == "" {
		http.Error(w, `invalid path – use /room/{roomId}`, http.StatusBadRequest)
		return
	}
	roomID := Sanitize(parts[1])
	if roomID == "" {
		http.Error(w, "invalid room ID", http.StatusBadRequest)
		return
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("conference: WebSocket upgrade failed: %v", err)
		return
	}

	rm := conference.getOrCreate(roomID)

	p := &peer{
		id:   generatePeerID(),
		conn: conn,
		send: make(chan []byte, 64),
	}

	if !rm.add(p) {
		sendJSON(p, SignalMessage{
			Type:    "error",
			Message: "room is full (maximum 5 users)",
		})
		// Flush the message before closing
		if msg, ok := <-p.send; ok {
			conn.WriteMessage(websocket.TextMessage, msg) //nolint:errcheck
		}
		conn.Close()
		log.Infof("conference: peer rejected – room %s is full", roomID)
		return
	}

	log.Infof("conference: peer %s joined room %s (%d/%d)",
		p.id, roomID, rm.size(), maxPeersPerRoom)

	// Tell the new peer its ID and who is already in the room.
	existing := rm.peerIDs(p.id)
	if existing == nil {
		existing = []string{}
	}
	sendJSON(p, SignalMessage{
		Type:          "joined",
		PeerID:        p.id,
		ExistingPeers: existing,
	})

	// Tell everyone else that a new peer has arrived.
	notify, _ := json.Marshal(SignalMessage{Type: "peer-joined", From: p.id})
	rm.broadcast(notify, p.id)

	go p.writePump()

	// Read pump – runs on this goroutine until the connection closes.
	defer func() {
		rm.remove(p.id)
		left, _ := json.Marshal(SignalMessage{Type: "peer-left", From: p.id})
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

		// Always stamp the sender's ID so the receiver knows who sent it.
		msg.From = p.id

		switch msg.Type {
		case "offer", "answer", "ice-candidate":
			if msg.To == "" {
				log.Warnf("conference: peer %s sent %q without a 'to' field", p.id, msg.Type)
				continue
			}
			out, _ := json.Marshal(msg)
			rm.relay(msg.To, out)

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
	mux.HandleFunc("/room/", conferenceHandler)

	log.Infof("Conference: WebSocket signaling server listening on %s", conferenceAddr)
	if err := http.ListenAndServe(conferenceAddr, mux); err != nil {
		log.Errorf("Conference: server error: %v", err)
	}
}
