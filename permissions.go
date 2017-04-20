package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/bwmarrin/discordgo"
	"io"
	"log"
	"os"
	"reflect"
	"sync"
)

type UserPermissions map[string]bool
type Permissions map[string]UserPermissions

type PermissionsManager struct {
	sync.RWMutex
	filepath    string
	permissions Permissions
}

func InitPermissions(filePath string) *PermissionsManager {
	pm := PermissionsManager{
		filepath:    filePath,
		permissions: make(Permissions),
	}

	f, err := os.OpenFile(filePath, os.O_RDONLY|os.O_CREATE, 0755)
	if err != nil {
		log.Println(err)
	}

	buf := bytes.NewBuffer(nil)
	io.Copy(buf, f)
	f.Close()

	json.Unmarshal(buf.Bytes(), pm.permissions)

	setperm := CommandConstructor{
		Names:             []string{"setperm"},
		Permission:        "setPermissions",
		DefaultPermission: false,
		NoArguments:       false,
		MinArguments:      3,
		MaxArguments:      3,
		RunFunc: func(raw []string, m *discordgo.MessageCreate, s *discordgo.Session) error {
			if len(m.Mentions) == 0 {
				return errors.New("No user specified")
			}

			if raw[2] != "true" && raw[2] != "false" {
				return errors.New("Invalid permission value")
			}

			pm.Set(m.Mentions[0].ID, raw[1], raw[2] == "true")

			s.ChannelMessageSend(m.ChannelID, "Permission set!")
			return nil
		},
	}

	RegisterCommands(&setperm)

	return &pm
}

/*func (perm Permissions) Load() {
	file, err := ioutil.ReadFile(filePath)
	if err != nil {
		log.Print(err)
	}
	if file != nil {
		json.Unmarshal(file, permissions)
	} else {
		permissions = make(Permissions)
	}
}*/

func (perm *PermissionsManager) Save() {
	perm.RLock()
	defer perm.RUnlock()

	text, err := json.Marshal(perm.permissions)
	if err != nil {
		log.Print(err)
	}

	f, err := os.OpenFile(perm.filepath, os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		log.Print(err)
	}

	f.Write(text)
	f.Close()
}

func (perm *PermissionsManager) Set(userID string, key string, value bool) error {
	cmd := permissionsCommand.FindByPermission(key)

	if reflect.DeepEqual(cmd, CommandConstructor{}) {
		return errors.New("No such permission")
	}

	perm.Lock()
	defer perm.Unlock()

	if perm.permissions[userID] == nil {
		perm.permissions[userID] = make(UserPermissions)
	}

	if cmd.DefaultPermission {
		perm.permissions[userID][key] = !value
	} else {
		perm.permissions[userID][key] = value
	}

	perm.Save()

	return nil
}

func (perm *PermissionsManager) Get(userID string, key string, commandDefault bool) bool {
	perm.RLock()
	defer perm.RUnlock()

	if commandDefault {
		return !perm.permissions[userID][key]
	} else {
		return perm.permissions[userID][key]
	}

}
