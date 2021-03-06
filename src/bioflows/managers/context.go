package managers

type ContextManager struct {
	stateManager StateManager
	remote bool
}

func (c *ContextManager) IsRemote() bool {
	return c.remote
}

func (c *ContextManager) SaveState(key string , state interface{}) error {
	return c.stateManager.SetStateByID(key,state)
}

func (c *ContextManager) Setup(config map[string]interface{}) error {
	remote , ok := config["remote"]
	if !ok {
		 remote = false
		 c.remote = false
	}
	if remote.(bool) {
		c.stateManager = &ClusterStateManager{}
		c.remote = true
	}else{
		c.stateManager = &LocalStateManager{}
		c.remote = false
	}
	return c.stateManager.Setup(config)
}

func (c *ContextManager) GetStateManager() StateManager{
	return c.stateManager
}
