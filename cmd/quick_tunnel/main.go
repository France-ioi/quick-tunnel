package main

import (
  "log"
  "net/http"
  "os"
  "strings"
  "github.com/gorilla/websocket"
)

type WsMessage struct {
    mt int
    msg []byte
}

type Relay struct {
    ctsChannel chan WsMessage
    stcChannel chan WsMessage
}

var relayMap = make(map[string]Relay)
var upgrader = websocket.Upgrader{}


func relayToRemote(local chan WsMessage, remote *websocket.Conn) {
    // Send messages to remote
    for {
        wsm, open := <-local
        if !open {
            break
        }
        err := remote.WriteMessage(wsm.mt, wsm.msg)
        if err != nil {
            log.Print("relay error: ", err)
            break
        }
    }
    remote.Close()
}

func remoteToRelay(remote *websocket.Conn, local chan WsMessage, isServer bool) {
    // Receive messages from remote
    for {
        mt, msg, err := remote.ReadMessage()
        if err != nil {
            log.Print("relay error: ", err)
            break
        }
        wsm := WsMessage{mt: mt, msg: msg}
        local <- wsm
    }
    remote.Close()
    if isServer {
        close(local)
    }
}

func clientHandler(w http.ResponseWriter, req *http.Request, code string, path string) {
    // Handle a request from a client
    thisRelay := relayMap[code]
    if thisRelay.ctsChannel == nil {
        http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
        return
    }

    // Upgrade HTTP to WebSocket
    c, err := upgrader.Upgrade(w, req, nil)
    if err != nil {
        log.Print("upgrader error: ", err)
        return
    }

    // Start relaying
    go relayToRemote(thisRelay.stcChannel, c)
    go remoteToRelay(c, thisRelay.ctsChannel, false)
}

func serverHandler(w http.ResponseWriter, req *http.Request, code string) {
    // Handle a request from a server

    // Create and register relay channels
    ctsc := make(chan WsMessage, 8)
    stcc := make(chan WsMessage, 8)
    relayMap[code] = Relay{ctsChannel: ctsc, stcChannel: stcc}

    // Upgrade HTTP to WebSocket
    c, err := upgrader.Upgrade(w, req, nil)
    if err != nil {
        log.Print("upgrader error: ", err)
        return
    }

    // Start relaying
    go relayToRemote(ctsc, c)
    go remoteToRelay(c, stcc, true)
}

func requestHandler(w http.ResponseWriter, req *http.Request) {
    // Receives all requests, routes to the server/client handlers
    pathSplit := strings.SplitN(req.URL.Path, "/", 4)

    // Request must be either
    //    /client/[code]/[target path]
    // or /server/[code]/
    if len(pathSplit) < 4 {
        http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
        return
    }

    if pathSplit[1] == "server" {
        serverHandler(w, req, pathSplit[2])
        return
    }

    if pathSplit[1] == "client" {
        clientHandler(w, req, pathSplit[2], "/" + pathSplit[3])
        return
    }

    // Malformed request
    http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
    return


}

func main() {
  var err error

  log.Printf("quick-tunnel is starting\n")

  var listen string = os.Getenv("LISTEN")
  if listen == "" {
    listen = ":4000"
  }
  log.Printf("starting tunnel on %s\n", listen)
  http.HandleFunc("/", requestHandler)
  err = http.ListenAndServe(listen, nil)
  if err != nil {
    log.Printf("fatal: %s\n", err)
  }
}
