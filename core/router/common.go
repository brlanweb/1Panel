package router

func commonGroups() []CommonRouter {
	return []CommonRouter{
		&BaseRouter{},
		&BackupRouter{},
		&LogRouter{},
		&SettingRouter{},
		&CommandRouter{},
		&GroupRouter{},
		&ClusterNodeRouter{},
		&ClusterWafRouter{},
		&ClusterSyncRouter{},
		&ScriptRouter{},
		&RuntimeDiagnosticsRouter{},
		&PanelMCPRouter{},
	}
}
