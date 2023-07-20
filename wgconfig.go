package main

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"
)

var reservedNames = []string{
	"CON", "PRN", "AUX", "NUL",
	"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
	"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
}

const (
	serviceNameForbidden = "$"
	netshellDllForbidden = "\\/:*?\"<>|\t"
	specialChars         = "/\\<>:\"|?*\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f\x00"
)

const (
	configFileSuffix            = ".conf.dpapi"
	configFileUnencryptedSuffix = ".conf"
)

func isReserved(name string) bool {
	if len(name) == 0 {
		return false
	}
	for _, reserved := range reservedNames {
		if strings.EqualFold(name, reserved) {
			return true
		}
		for i := len(name) - 1; i >= 0; i-- {
			if name[i] == '.' {
				if strings.EqualFold(name[:i], reserved) {
					return true
				}
				break
			}
		}
	}
	return false
}

func hasSpecialChars(name string) bool {
	return strings.ContainsAny(name, specialChars) || strings.ContainsAny(name, netshellDllForbidden) || strings.ContainsAny(name, serviceNameForbidden)
}

var allowedNameFormat = regexp.MustCompile("^[a-zA-Z0-9_=+.-]{1,32}$")

func TunnelNameIsValid(name string) bool {
	// Aside from our own restrictions, let's impose the Windows restrictions first
	if isReserved(name) || hasSpecialChars(name) {
		return false
	}
	return allowedNameFormat.MatchString(name)
}

func NameFromPath(path string) (string, error) {
	name := filepath.Base(path)
	if !((len(name) > len(configFileSuffix) && strings.HasSuffix(name, configFileSuffix)) ||
		(len(name) > len(configFileUnencryptedSuffix) && strings.HasSuffix(name, configFileUnencryptedSuffix))) {
		return "", errors.New("path must end in either " + configFileSuffix + " or " + configFileUnencryptedSuffix)
	}
	if strings.HasSuffix(path, configFileSuffix) {
		name = strings.TrimSuffix(name, configFileSuffix)
	} else {
		name = strings.TrimSuffix(name, configFileUnencryptedSuffix)
	}
	if !TunnelNameIsValid(name) {
		return "", errors.New("tunnel name is not valid")
	}
	return name, nil
}
