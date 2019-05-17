package main

import (
	"fmt"
	"net/http"
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/satori/go.uuid"
	_"log"
)

type ClientManager struct {
    clients    map[*Client]bool
    broadcast  chan []byte
    register   chan *Client
    unregister chan *Client
}

type Client struct {
	id     string
	username string
    socket *websocket.Conn
    send   chan []byte
}

type Message struct {
	Type    string `json:"type,omitempty"`
	Sender    string `json:"sender,omitempty"`
	Sender_id   string `json:"sender_id,omitempty"`
    Recipient string `json:"recipient,omitempty"`
    Content   string `json:"content,omitempty"`
}

var manager = ClientManager{
    broadcast:  make(chan []byte),
    register:   make(chan *Client),
    unregister: make(chan *Client),
    clients:    make(map[*Client]bool),
}

func (manager *ClientManager) start() {
    for {
        select {
		case conn := <-manager.register:
            manager.clients[conn] = true
            jsonMessage, _ := json.Marshal(&Message{Content: "A socket has connected.", Sender: conn.username, Type: "New", Sender_id: conn.id})
            manager.send(jsonMessage, conn)
        case conn := <-manager.unregister:
            if _, ok := manager.clients[conn]; ok {
                close(conn.send)
                delete(manager.clients, conn)
                jsonMessage, _ := json.Marshal(&Message{Content: "A socket has disconnected.", Sender: conn.username, Type: "Leave", Sender_id: conn.id})
                manager.send(jsonMessage, conn)
            }
        case message := <-manager.broadcast:
            for conn := range manager.clients {
                select {
                case conn.send <- message:
                default:
                    close(conn.send)
                    delete(manager.clients, conn)
                }
            }
        }
    }
}

func (manager *ClientManager) send(message []byte, ignore *Client) {
    for conn := range manager.clients {
        if conn != ignore {
            conn.send <- message
        }
    }
}

func (c *Client) read() {
    defer func() {
        manager.unregister <- c
        c.socket.Close()
    }()

    for {
		_, message, err := c.socket.ReadMessage()
		
        if err != nil {
            manager.unregister <- c
            c.socket.Close()
            break
        }
        jsonMessage, _ := json.Marshal(&Message{Sender_id: c.id, Sender: c.username, Content: string(message), Type: "Msg"})
        manager.broadcast <- jsonMessage
    }
}

func (c *Client) write() {
    defer func() {
        c.socket.Close()
    }()

    for {
        select {
		case message, ok := <-c.send:
            if !ok {
                c.socket.WriteMessage(websocket.CloseMessage, []byte{})
                return
            }

            c.socket.WriteMessage(websocket.TextMessage, message)
        }
    }
}

func main() {
    fmt.Println("Starting application...")
	go manager.start()
	http.HandleFunc("/", serveHome)
    http.HandleFunc("/ws", wsPage)
    http.ListenAndServe(":8080", nil)
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.ServeFile(w, r, "index.html")
}

func wsPage(res http.ResponseWriter, req *http.Request) {
    conn, error := (&websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}).Upgrade(res, req, nil)
    if error != nil {
        http.NotFound(res, req)
        return
	}

	id, _ := uuid.NewV4()
	client := &Client{id: id.String(),username: req.URL.Query().Get("username"), socket: conn, send: make(chan []byte)}

    manager.register <- client

    go client.read()
    go client.write()
}
