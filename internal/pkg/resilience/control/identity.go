package control

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
)

type InstanceIdentity struct {
	Component  string `json:"component"`
	InstanceID string `json:"instance_id"`
	Generation string `json:"generation"`
}

func ResolveInstanceIdentity(component, configured string) InstanceIdentity {
	instanceID := strings.TrimSpace(configured)
	if instanceID == "" {
		instanceID, _ = os.Hostname()
	}
	if instanceID == "" {
		instanceID = "instance-" + randomToken()
	}
	return InstanceIdentity{
		Component:  component,
		InstanceID: instanceID,
		Generation: randomToken(),
	}
}

func randomToken() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(raw[:])
}
