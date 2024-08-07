package main

import (
	"encoding/json"
	"io"
	"os"
	"sync"

	log "github.com/sirupsen/logrus"
)

var (
	KeyStore map[string]string
	KeyLock  sync.Mutex
)

func KeyInitialize() {
	KeyLock.Lock()
	defer KeyLock.Unlock()

	KeyStore = make(map[string]string)
}

func KeyLookup(key string) (string, bool) {
	KeyLock.Lock()
	defer KeyLock.Unlock()
	value, found := KeyStore[key]
	return value, found
}

func KeyAdd(public string, private string) {
	KeyLock.Lock()
	defer KeyLock.Unlock()

	KeyStore[public] = private

}

func KeyDelete(key string) {
	KeyLock.Lock()
	defer KeyLock.Unlock()

	delete(KeyStore, key)
}

func KeySave() error {

	KeyLock.Lock()
	defer KeyLock.Unlock()

	file, err := os.OpenFile(GetDataPath()+"keys.keys", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if file != nil {
		defer file.Close()
	}
	if err != nil {
		log.Errorf("Error opening keys.keys for write: %v", err)
		return err
	}
	bytes, err := json.Marshal(KeyStore)
	if err != nil {
		log.Errorf("Error marshalling json: %v", err)
	}
	_, err = file.Write(bytes)

	return err
}

func KeyLoad() error {
	KeyLock.Lock()
	defer KeyLock.Unlock()

	file, err := os.Open(GetDataPath() + "keys.keys")
	if err != nil {
		log.Errorf("Error opening keys.keys for read: %v", err)
		return err
	}

	bytes, err := io.ReadAll(file)
	file.Close()

	if err != nil {
		log.Errorf("Error reading keys.keys: %v", err)
		return err
	}

	err = json.Unmarshal(bytes, &KeyStore)
	if err != nil {
		log.Errorf("Error unmarshalling json: %v", err)
	}

	return err
}
