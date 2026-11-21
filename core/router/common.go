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
		&ClusterMonitorRouter{},
		&ClusterSyncRouter{},
		&ClusterDbReplicationRouter{},
		&ScriptRouter{},
		&RuntimeDiagnosticsRouter{},
		&PanelMCPRouter{},
	}
}
