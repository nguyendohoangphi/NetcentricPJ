package main

import (
	"encoding/json"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Player struct {
	Level    int    `json:"level"`
	EXP      int    `json:"exp"`
	Password string `json:"password"`
	Username string `json:"username"`
}

type MatchHistoryEntry struct {
	Timestamp string `json:"timestamp"`
	Player1   string `json:"player1"`
	Player2   string `json:"player2"`
	Winner    string `json:"winner"`
	Result    string `json:"result"`
}

type WsClient struct {
	conn     *websocket.Conn
	ready    bool
	username string
	loggedIn bool
}

var clients []*WsClient
var allPlayers = make(map[string]Player)
var mu sync.Mutex

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func handleWs(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}
	client := &WsClient{conn: conn, ready: false, loggedIn: false}
	loadPlayers()
	clients = append(clients, client)

	for {
		var msg map[string]interface{}
		err := conn.ReadJSON(&msg)
		if err != nil {
			log.Println("Read error:", err)
			break
		}
		switch msg["type"] {
		case "login":
			u := msg["username"].(string)
			p := msg["password"].(string)
			player, ok := allPlayers[u]
			if !ok || player.Password != p {
				client.conn.WriteJSON(map[string]interface{}{"type": "login_error", "message": "Sai tên đăng nhập hoặc mật khẩu."})
				continue
			}
			client.loggedIn = true
			client.username = u
			client.conn.WriteJSON(map[string]interface{}{"type": "login_success", "username": u, "level": player.Level, "exp": player.EXP})

		case "ready":
			if !client.loggedIn {
				client.conn.WriteJSON(map[string]interface{}{"type": "error", "message": "Bạn chưa đăng nhập."})
				continue
			}
			client.ready = true
			checkStartGame()

		case "deploy":
			if !client.loggedIn {
				client.conn.WriteJSON(map[string]interface{}{"type": "error", "message": "Bạn chưa đăng nhập."})
				continue
			}
			broadcast(map[string]interface{}{
				"type":   "action",
				"time":   time.Now().Format("15:04:05"),
				"detail": client.username + " triển khai: " + msg["troop"].(string),
			})

		case "ping":
			client.conn.WriteJSON(map[string]interface{}{"type": "pong"})
		}
	}
}

func checkStartGame() {
	if len(clients) >= 2 && clients[0].ready && clients[1].ready {
		broadcast(map[string]interface{}{"type": "start"})
		go simulateBattle(clients[0], clients[1])
	}
}

func simulateBattle(p1, p2 *WsClient) {
	time.Sleep(3 * time.Minute)

	winner := p1
	loser := p2
	if rand.Intn(2) == 1 {
		winner, loser = p2, p1
	}

	winner.conn.WriteJSON(map[string]interface{}{"type": "result", "result": "Bạn Thắng!"})
	loser.conn.WriteJSON(map[string]interface{}{"type": "result", "result": "Bạn Thua!"})

	saveMatchHistory(MatchHistoryEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Player1:   p1.username,
		Player2:   p2.username,
		Winner:    winner.username,
		Result:    "Win/Lose",
	})

	updateExp(winner.username, 30)
	updateExp(loser.username, 10)

	p1.ready = false
	p2.ready = false
}

func broadcast(msg map[string]interface{}) {
	for _, c := range clients {
		c.conn.WriteJSON(msg)
	}
}

func updateExp(username string, gain int) {
	mu.Lock()
	defer mu.Unlock()
	loadPlayers()
	p := allPlayers[username]
	p.EXP += gain
	for {
		req := int(100 * math.Pow(1.1, float64(p.Level)))
		if p.EXP >= req {
			p.EXP -= req
			p.Level++
		} else {
			break
		}
	}
	allPlayers[username] = p
	savePlayers()
}

func loadPlayers() {
	f, err := os.Open("res.json")
	if err != nil {
		return
	}
	defer f.Close()
	json.NewDecoder(f).Decode(&allPlayers)
}

func savePlayers() {
	f, err := os.Create("res.json")
	if err != nil {
		return
	}
	defer f.Close()
	json.NewEncoder(f).Encode(allPlayers)
}

func saveMatchHistory(entry MatchHistoryEntry) {
	var history []MatchHistoryEntry
	f, err := os.Open("history.json")
	if err == nil {
		_ = json.NewDecoder(f).Decode(&history)
		f.Close()
	}
	history = append(history, entry)
	f2, err := os.Create("history.json")
	if err != nil {
		return
	}
	defer f2.Close()
	json.NewEncoder(f2).Encode(history)
}

func StartWebSocketServer() {
	http.HandleFunc("/ws", handleWs)
	log.Println("WebSocket Server đang chạy tại :9001/ws")
	log.Fatal(http.ListenAndServe(":9001", nil))
}
