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
	mu       sync.Mutex
}

type Battle struct {
	p1, p2     *WsClient
	cancelChan chan struct{}
}

var activeBattle *Battle

var clients []*WsClient
var allPlayers = make(map[string]Player)
var clientsMu sync.Mutex
var regenTimers = make(map[string]*time.Ticker)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

var towerHp = map[string]int{
	"player1": 100,
	"player2": 100,
}
var mana = map[string]int{
	"player1": 5,
	"player2": 5,
}

var troopDamage = map[string]int{
	"Pawn": 3, "Bishop": 4, "Rook": 5,
	"Knight": 5, "Prince": 6, "Queen": 5,
}
var troopCost = map[string]int{
	"Pawn": 3, "Bishop": 4, "Rook": 5,
	"Knight": 5, "Prince": 6, "Queen": 5,
}

func handleWs(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil { /*…*/
	}
	client := &WsClient{conn: conn}

	// 1) Thêm client
	clientsMu.Lock()
	clients = append(clients, client)
	clientsMu.Unlock()

	addr := conn.RemoteAddr().String()
	log.Printf("[DEBUG] ↔️  New WS connection: %s\n", addr)

	// 2) Khi disconnect
	defer func() {
		conn.Close()

		clientsMu.Lock()
		for i, c := range clients {
			if c == client {
				clients = append(clients[:i], clients[i+1:]...)
				break
			}
		}
		clientsMu.Unlock()

		if client.loggedIn {
			log.Printf("[DEBUG] 📴 Client disconnected: %s (user=%s), releasing role\n", addr, client.username)

			broadcast(map[string]interface{}{
				"type":     "role_released",
				"username": client.username,
			})
			stopManaRegen(client.username)

			// ❗ Reset người còn lại
			for _, c := range clients {
				if c != client && c.loggedIn {

					mana[c.username] = 5
					c.ready = false
					stopManaRegen(c.username)
					safeWriteJSON(c, map[string]interface{}{
						"type":   "force_reset",
						"reason": client.username + " has left the match.",
					})
				}
			}

			// ❗ Huỷ simulateBattle nếu cần
			if activeBattle != nil && (activeBattle.p1 == client || activeBattle.p2 == client) {
				close(activeBattle.cancelChan)
				activeBattle = nil
			}
		}
	}()

	for {
		var msg map[string]interface{}
		if err := client.conn.ReadJSON(&msg); err != nil {
			log.Println("[ERROR] ReadJSON:", err)
			break
		}

		switch msg["type"] {
		case "login":
			u, _ := msg["username"].(string)
			p, _ := msg["password"].(string)

			// 1) Kiểm tra duplicate
			duplicate := false
			for _, cl := range clients {
				if cl.loggedIn && cl.username == u {
					duplicate = true
					break
				}
			}
			if duplicate {
				log.Printf("[WARN] ⚠️ Login blocked (duplicate): %s\n", u)
				client.conn.WriteJSON(map[string]interface{}{
					"type":    "login_error",
					"message": "This user has logged in another tab!.",
				})
				continue // >>> continue outer for -> bỏ qua rest of case
			}

			// 2) Xác thực credentials
			player, ok := allPlayers[u]
			if !ok || player.Password != p {
				log.Printf("[WARN] ❌ Invalid credentials for %s\n", u)
				client.conn.WriteJSON(map[string]interface{}{
					"type":    "login_error",
					"message": "Sai tên đăng nhập hoặc mật khẩu.",
				})
				continue
			}

			// 3) Đăng nhập thành công
			client.loggedIn = true
			client.username = u
			log.Printf("[INFO] ✅ Login success: %s\n", u)
			client.conn.WriteJSON(map[string]interface{}{
				"type":     "login_success",
				"username": u,
				"level":    player.Level,
				"exp":      player.EXP,
			})

			// 4) Broadcast role_selected
			for _, cl := range clients {
				cl.conn.WriteJSON(map[string]interface{}{
					"type":     "role_selected",
					"username": u,
				})
			}

		case "ready":
			if !client.loggedIn {
				client.conn.WriteJSON(map[string]interface{}{
					"type":    "error",
					"message": "Bạn chưa đăng nhập.",
				})
				continue
			}
			client.ready = true
			log.Printf("[DEBUG] %s is ready\n", client.username)
			checkStartGame()

		case "deploy":
			if !client.loggedIn {
				client.conn.WriteJSON(map[string]interface{}{
					"type":    "error",
					"message": "Bạn chưa đăng nhập.",
				})
				continue
			}

			troop, ok := msg["troop"].(string)
			if !ok {
				client.conn.WriteJSON(map[string]interface{}{
					"type":    "error",
					"message": "Thiếu tên troop.",
				})
				continue
			}

			user := client.username
			cost := troopCost[troop]
			damage := troopDamage[troop]

			if mana[user] < cost {
				client.conn.WriteJSON(map[string]interface{}{
					"type":    "error",
					"message": "Dont have enough mana!.",
				})
				continue
			}

			// Trừ mana người dùng
			mana[user] -= cost

			// Tìm đối thủ
			var opponent string
			for _, c := range clients {
				if c != client && c.loggedIn {
					opponent = c.username
					break
				}
			}
			if opponent == "" {
				client.conn.WriteJSON(map[string]interface{}{
					"type":    "error",
					"message": "No opponent is online.",
				})
				continue
			}

			// Trừ HP đối thủ
			if troop == "Queen" {
				// 👑 Hồi máu cho chính người chơi
				towerHp[user] += 3
				if towerHp[user] > 100 {
					towerHp[user] = 100
				}
			} else {
				// ⚔️ Gây damage cho đối thủ như bình thường
				towerHp[opponent] -= damage
				if towerHp[opponent] < 0 {
					towerHp[opponent] = 0
				}
			}

			// Gửi thông báo cho cả hai
			now := time.Now().Format("15:04:05")
			detail := user + " deployed: " + troop

			broadcast(map[string]interface{}{
				"type":       "action",
				"time":       now,
				"detail":     detail,
				"actor":      user,
				"troop":      troop,
				"actorMana":  mana[user],
				"actorHp":    towerHp[user],
				"targetMana": mana[opponent],
				"targetHp":   towerHp[opponent],
			})

		case "ping":
			client.conn.WriteJSON(map[string]interface{}{"type": "pong"})
			log.Printf("[DEBUG] Pong to the client!.")

		default:
			log.Printf("[WARN] Unknown message type: %v\n", msg["type"])
		}
	}
}
func checkStartGame() {
	log.Println("[DEBUG] ▶️  checkStartGame triggered")
	if len(clients) >= 2 && clients[0].ready && clients[1].ready {
		broadcast(map[string]interface{}{"type": "start"})
		startManaRegen(clients[0].username)
		startManaRegen(clients[1].username)
		go simulateBattle(clients[0], clients[1])
	}
}

func simulateBattle(p1, p2 *WsClient) {
	log.Printf("[DEBUG] ⏳ simulateBattle running for %s vs %s\n", p1.username, p2.username)

	for i := 0; i < 180; i++ { // 180 giây
		time.Sleep(1 * time.Second)

		// Nếu một trong hai player đã out
		if !isClientConnected(p1.username) || !isClientConnected(p2.username) {
			log.Printf("[WARN] 🛑 Trận đấu bị huỷ do 1 người thoát.\n")
			return
		}
	}

	// Vẫn còn đủ người chơi → xử lý kết quả
	winner := p1
	loser := p2
	if rand.Intn(2) == 1 {
		winner, loser = p2, p1
	}

	safeWriteJSON(winner, map[string]interface{}{"type": "result", "result": "You win!"})
	safeWriteJSON(loser, map[string]interface{}{"type": "result", "result": "You lose!"})

	saveMatchHistory(MatchHistoryEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Player1:   p1.username,
		Player2:   p2.username,
		Winner:    winner.username,
		Result:    "Win/Lose",
	})

	updateExp(winner.username, 30)
	updateExp(loser.username, 10)

	stopManaRegen(p1.username)
	stopManaRegen(p2.username)
	p1.ready = false
	p2.ready = false
}

func isClientConnected(username string) bool {
	for _, c := range clients {
		if c.loggedIn && c.username == username {
			return true
		}
	}
	return false
}

func safeWriteJSON(c *WsClient, v interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	err := c.conn.WriteJSON(v)
	if err != nil {
		log.Printf("[ERROR] ❌ WriteJSON to %s failed: %v\n", c.username, err)
	}
}

func broadcast(msg map[string]interface{}) {
	log.Printf("[DEBUG] 🔊 Broadcasting: %v\n", msg)
	for _, c := range clients {
		c.conn.WriteJSON(msg)
	}

}

func updateExp(username string, gain int) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
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
		// lần đầu: tạo 2 tài khoản mặc định
		allPlayers["player1"] = Player{Username: "player1", Password: "1234", Level: 1, EXP: 0}
		allPlayers["player2"] = Player{Username: "player2", Password: "1234", Level: 1, EXP: 0}
		savePlayers()
		log.Println("[INFO] res.json not found — created default players")
		return
	}
	defer f.Close()
	json.NewDecoder(f).Decode(&allPlayers)
	log.Println("[INFO] Loaded players from res.json")
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

func startManaRegen(username string) {
	stopManaRegen(username) // dọn dẹp cũ nếu có

	ticker := time.NewTicker(1 * time.Second)
	regenTimers[username] = ticker

	go func() {
		for range ticker.C {
			if mana[username] < 10 {
				mana[username]++
				// Gửi cập nhật chỉ cho chính người chơi đó
				for _, c := range clients {
					if c.loggedIn && c.username == username {
						c.conn.WriteJSON(map[string]interface{}{
							"type":     "mana_update",
							"username": username, // 👈 THÊM DÒNG NÀY
							"mana":     mana[username],
							"tower_hp": towerHp[username],
						})
						break
					}
				}
			}
		}
	}()
}

func stopManaRegen(username string) {
	if ticker, ok := regenTimers[username]; ok {
		ticker.Stop()
		delete(regenTimers, username)
	}
}

func StartWebSocketServer() {
	loadPlayers()
	http.HandleFunc("/ws", handleWs)
	log.Println("WebSocket Server đang chạy tại :9001/ws")
	log.Fatal(http.ListenAndServe(":9001", nil))
}
