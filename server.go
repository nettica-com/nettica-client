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
	ServersMutex.Lock()
	defer ServersMutex.Unlock()

	server := &Server{
		Name:     CleanupName(name),
		Path:     GetServerPath(name),
		Config:   config,
		Running:  make(chan bool),
		Shutdown: false,
	}

	Servers[server.Path] = server
	return server
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
			if file.Name() == "keys.json" {
				continue
			}

			if strings.HasSuffix(file.Name(), "-service-host.json") {
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

			if server.Config.Device != nil && server.Config.Device.Server != "" {
				server.Name = server.Config.Device.Server
			} else {
				server.Name = strings.TrimSuffix(file.Name(), ".json")
			}

			if server.Config.Device != nil {
				switch server.Config.Device.Logging {
				case "debug":
					log.SetLevel(log.DebugLevel)
				case "info":
					log.SetLevel(log.InfoLevel)
				case "error":
					log.SetLevel(log.ErrorLevel)
				default:
					log.SetLevel(log.FatalLevel)
				}
			}

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

	path := server.Path
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
	server.Body = data
}

func CleanupName(name string) string {

	// remove the https:// or http:// from the name if it exists
	if strings.ToLower(name[:8]) == "https://" {
		name = name[8:]
	} else if strings.ToLower(name[:7]) == "http://" {
		name = name[7:]
	}

	return Sanitize(name)
}

func GetServerPath(name string) string {

	name = CleanupName(name)

	return filepath.Join(GetDataPath(), name+".json")
}

func RemoveServer(server *Server) {
	ServersMutex.Lock()
	defer ServersMutex.Unlock()
	log.Infof("Removing server %s (%s)", server.Name, server.Path)

	err := os.Remove(server.Path)
	if err != nil {
		log.Errorf("Failed to remove file %s: %v", server.Path, err)
	}

	delete(Servers, server.Path)

	server.Shutdown = true

	log.Info("Server removed")

}
