package control

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

type InstanceIdentity struct {
	Component  string `json:"component"`
	InstanceID string `json:"instance_id"`
	Generation string `json:"generation"`
}

var readRandom = rand.Read

func ResolveInstanceIdentity(component, configured string) (InstanceIdentity, error) {
	instanceID := strings.TrimSpace(configured)
	if instanceID == "" {
		instanceID, _ = os.Hostname()
	}
	if instanceID == "" {
		token, err := randomToken()
		if err != nil {
			return InstanceIdentity{}, fmt.Errorf("generate resilience instance id: %w", err)
		}
		instanceID = "instance-" + token
	}
	generation, err := randomToken()
	if err != nil {
		return InstanceIdentity{}, fmt.Errorf("generate resilience instance generation: %w", err)
	}
	return InstanceIdentity{
		Component:  component,
		InstanceID: instanceID,
		Generation: generation,
	}, nil
}

func randomToken() (string, error) {
	var raw [8]byte
	if _, err := readRandom(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
