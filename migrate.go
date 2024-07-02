package main

import (
	"io"
	"os"
	"runtime"
	"strings"

	log "github.com/sirupsen/logrus"
)

func Migrate() {

	log.Infof("*** Migrate ***")

	if !cfg.loaded {
		return
	}

	original := GetDataPath() + "original\\"

	err := os.MkdirAll(GetDataPath()+"original", 0755)
	if err != nil {
		log.Error("Failed to create original directory: ", err)
	}

	err = Copy(original+"nettica.json", GetDataPath()+"nettica.json")
	if err != nil {
		log.Error("Failed to copy nettica.json: ", err)
	}

	err = Copy(original+"nettica.conf", GetDataPath()+"nettica.conf")
	if err != nil {
		log.Error("Failed to copy nettica.conf: ", err)
	}

	err = Copy(original+"nettica-service-host.json", GetDataPath()+"nettica-service-host.json")
	if err != nil {
		log.Error("Failed to copy nettica-service-host.json: ", err)
	}

	// temporarily copy the file instead of renaming it so old agent can still read it
	//err = os.Rename(GetDataPath()+"nettica.json", GetDataPath()+"my.nettica.com.json")
	err = Copy(GetDataPath()+"my.nettica.com.json", GetDataPath()+"nettica.json")
	if err != nil {
		log.Error("Failed to rename nettica.json: ", err)
	}

	err = os.Rename(GetDataPath()+"nettica-service-host.json", GetDataPath()+"my.nettica.com-service-host.json")
	if err != nil {
		log.Error("Failed to rename nettica-service-host.json: ", err)
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
		return err
	}

	// Close the destination file
	err = dFile.Close()
	if err != nil {
		return err
	}

	return nil
}
