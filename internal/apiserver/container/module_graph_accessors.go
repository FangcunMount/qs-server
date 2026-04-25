package container

import actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"

func (c *Container) actorTesteeAccessService() actorAccessApp.TesteeAccessService {
	if c == nil || c.ActorModule == nil {
		return nil
	}
	return c.ActorModule.TesteeAccessService
}
