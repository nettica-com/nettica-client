package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/nettica-com/nettica-admin/model"
	log "github.com/sirupsen/logrus"
)

var (
	Servers      = make(map[string]*Server)
	ServersMutex = &sync.Mutex{}
)

func NewServer(name string, config model.Message) *Server {
	return &Server{
		Name:    name,
		Path:    GetServerPath(name),
		Config:  config,
		Running: make(chan bool),
	}
}

func LoadServers() error {

	// If we already have servers (from environment variables), don't load from directory
	if len(Servers) > 0 {
		return nil
	}

	ServersMutex.Lock()
	defer ServersMutex.Unlock()

	// Find all the .json files in the config directory (/etc/nettica)

	dir := GetDataPath()
	files, err := os.ReadDir(dir)
	if err != nil {
		log.Errorf("Failed to read config directory: %v", err)
		return err
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if filepath.Ext(file.Name()) == ".json" {
			if file.Name() == "nettica.json" {
				continue
			}
			path := filepath.Join(dir, file.Name())

			data, err := os.ReadFile(path)
			if err != nil {
				log.Printf("Failed to read file %s: %v", path, err)
				continue
			}
			var server Server
			if err := json.Unmarshal(data, &server.Config); err != nil {
				log.Printf("Failed to unmarshal JSON from file %s: %v", path, err)
				continue
			}
			server.Name = server.Config.Device.Server
			server.Path = path
			server.Body = data
			server.Running = make(chan bool)

			Servers[path] = &server
		}
	}

	return nil
}

func SaveServer(server *Server) {
	ServersMutex.Lock()
	defer ServersMutex.Unlock()

	path := GetServerPath(server.Name)
	// Although we have a Body field, the JSON is
	// the source of truth for the config.
	data, err := json.Marshal(server.Config)
	if err != nil {
		log.Printf("Failed to marshal JSON: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("Failed to write file %s: %v", path, err)
	}
}

func GetServerPath(name string) string {
	// remove the https:// or http:// from the name if it exists
	if strings.ToLower(name[:8]) == "https://" {
		name = name[8:]
	} else if strings.ToLower(name[:7]) == "http://" {
		name = name[7:]
	}

	return filepath.Join(GetDataPath(), name+".json")
}

func RemoveServer(server *Server) {
	ServersMutex.Lock()
	defer ServersMutex.Unlock()

	server.Running <- false
	if err := os.Remove(server.Path); err != nil {
		log.Printf("Failed to remove file %s: %v", server.Path, err)
	}

	delete(Servers, server.Path)

}