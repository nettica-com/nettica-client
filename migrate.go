package main

import (
	"encoding/json"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/nettica-com/nettica-admin/model"
	log "github.com/sirupsen/logrus"
)

func Migrate() {

	log.Infof("*** Migrate ***")

	if !cfg.loaded {
		return
	}

	if _, err := os.Stat(GetDataPath() + "nettica.json"); err != nil {
		if _, err := os.Stat(GetDataPath() + "nettica.conf"); err != nil {
			if _, err := os.Stat(GetDataPath() + "keys.json"); err != nil {
				if _, err := os.Stat(GetDataPath() + "nettica-service-host.json"); err != nil {
					log.Info("No files to migrate")
					return
				}
			}
		}
	}

	original := GetDataPath() + "original\\"

	stat, err := os.Stat(GetDataPath() + "original")
	if err == nil && stat.IsDir() {
		log.Info("Original directory already exists, skipping migration")
		return
	}

	err = os.MkdirAll(GetDataPath()+"original", 0755)
	if err != nil {
		log.Error("Failed to create original directory: ", err)
	}

	// temporarily copy the file instead of renaming it so old agent can still read it
	//err = os.Rename(GetDataPath()+"nettica.json", GetDataPath()+"my.nettica.com.json")
	var msg model.Message
	data, err := os.ReadFile(GetDataPath() + "nettica.json")
	if err != nil {
		log.Printf("Failed to read file %s: %v", "nettica.json", err)
	} else {
		err = json.Unmarshal(data, &msg)
		if err == nil {
			name := msg.Device.Server
			name = strings.Replace("https://", "", name, -1)
			name = strings.Replace("http://", "", name, -1)
			err = os.WriteFile(GetDataPath()+name+".json", data, 0644)
			if err != nil {
				log.Errorf("Failed to create %s.json: %v", name, err)
			}
		} else {
			log.Errorf("Failed to unmarshal %s: %v", "nettica.json", err)
		}
	}

	err = Copy(original+"nettica.json", GetDataPath()+"nettica.json")
	if err != nil {
		log.Error("Failed to copy nettica.json: ", err)
	} else {
		err = os.Remove(GetDataPath() + "nettica.json")
		if err != nil {
			log.Error("Failed to remove nettica.json: ", err)
		}
	}

	err = Copy(original+"keys.json", GetDataPath()+"keys.json")
	if err != nil {
		log.Error("Failed to copy keys.json: ", err)
	}

	err = Copy(original+"nettica.conf", GetDataPath()+"nettica.conf")
	if err != nil {
		log.Error("Failed to copy nettica.conf: ", err)
	} else {
		err = os.Remove(GetDataPath() + "nettica.conf")
		if err != nil {
			log.Error("Failed to remove nettica.conf: ", err)
		}
	}

	err = Copy(original+"nettica-service-host.json", GetDataPath()+"nettica-service-host.json")
	if err != nil {
		log.Error("Failed to copy nettica-service-host.json: ", err)
	}

	err = os.Rename(GetDataPath()+"nettica-service-host.json", GetDataPath()+"my.nettica.com-service-host.json")
	if err != nil {
		log.Error("Failed to rename nettica-service-host.json: ", err)
	}

	err = os.Rename(GetDataPath()+"keys.json", GetDataPath()+"keys.keys")
	if err != nil {
		log.Error("Failed to rename keys.json: ", err)
	}

	log.Info("*** Migrate Done ***")
}

func Copy(dst string, src string) error {

	// If platform is windows, fix the slashes
	if runtime.GOOS == "windows" {
		dst = strings.ReplaceAll(dst, "/", "\\")
		src = strings.ReplaceAll(src, "/", "\\")
	} else {
		dst = strings.ReplaceAll(dst, "\\", "/")
		src = strings.ReplaceAll(src, "\\", "/")
	}

	// Open the source file
	sFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sFile.Close()

	// Create the destination file
	dFile, err := os.Create(dst)
	if err != nil {
		return err
	}

	// Copy the file
	_, err = io.Copy(dFile, sFile)
	if err != nil {
		dFile.Close()
		return err
	}

	// Close the destination file
	err = dFile.Close()
	if err != nil {
		return err
	}

	return nil
}
