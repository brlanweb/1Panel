package menutree

import "github.com/1Panel-dev/1Panel/core/app/dto"

const (
	xpackMenuID    = "11"
	xpackMenuLabel = "Xpack-Menu"
)

func PreserveMissingMenus(menus, fallback []dto.ShowMenu) ([]dto.ShowMenu, bool) {
	updated := cloneMenus(menus)
	changed := preserveMissingMenus(&updated, &updated, fallback)
	return updated, changed
}

func preserveMissingMenus(root, current *[]dto.ShowMenu, fallback []dto.ShowMenu) bool {
	changed := false
	for _, previous := range fallback {
		menuIndex := findMatchingMenu(*current, previous)
		if menuIndex < 0 {
			if containsMenu(*root, previous) {
				continue
			}
			*current = append(*current, cloneMenu(previous))
			changed = true
			continue
		}
		if preserveMissingMenus(root, &(*current)[menuIndex].Children, previous.Children) {
			changed = true
		}
	}
	return changed
}

// ReconcileHideMenuIntegrity normalises a stored HideMenu tree. This fork
// has no paid "advanced features" (Xpack) pages at all, so the Xpack-Menu
// subtree (and the retired Upage / XApp entries that used to be forced into
// it) is always removed; it would otherwise only show dead links.
func ReconcileHideMenuIntegrity(menus, _ []dto.ShowMenu) ([]dto.ShowMenu, bool) {
	updated := cloneMenus(menus)
	changed := false
	for _, identity := range []dto.ShowMenu{retiredUpageMenuIdentity(), retiredXAppMenuIdentity()} {
		var removed bool
		updated, removed = removeSystemMenus(updated, identity)
		changed = changed || removed
	}
	var removed bool
	updated, removed = RemoveXpackMenu(updated)
	return updated, changed || removed
}

// RemoveXpackMenu drops the top-level "advanced features" menu with all of
// its children.
func RemoveXpackMenu(menus []dto.ShowMenu) ([]dto.ShowMenu, bool) {
	filtered := make([]dto.ShowMenu, 0, len(menus))
	changed := false
	for _, menu := range menus {
		if menu.ID == xpackMenuID || menu.Label == xpackMenuLabel {
			changed = true
			continue
		}
		filtered = append(filtered, menu)
	}
	return filtered, changed
}

func retiredXAppMenuIdentity() dto.ShowMenu {
	return dto.ShowMenu{
		ID:    "118",
		Label: "XApp",
		Path:  "/xpack/app",
	}
}

func retiredUpageMenuIdentity() dto.ShowMenu {
	return dto.ShowMenu{
		ID:    "119",
		Label: "Upage",
		Path:  "/xpack/upage",
	}
}

func findMatchingMenu(menus []dto.ShowMenu, target dto.ShowMenu) int {
	if target.ID != "" {
		for i := range menus {
			if menus[i].ID == target.ID {
				return i
			}
		}
	}
	if target.Label != "" {
		for i := range menus {
			if menus[i].Label == target.Label {
				return i
			}
		}
	}
	if target.Path != "" {
		for i := range menus {
			if menus[i].Path == target.Path {
				return i
			}
		}
	}
	return -1
}

func containsMenu(menus []dto.ShowMenu, target dto.ShowMenu) bool {
	if findMatchingMenu(menus, target) >= 0 {
		return true
	}
	for i := range menus {
		if containsMenu(menus[i].Children, target) {
			return true
		}
	}
	return false
}

func removeSystemMenus(menus []dto.ShowMenu, identity dto.ShowMenu) ([]dto.ShowMenu, bool) {
	if menus == nil {
		return nil, false
	}
	filtered := make([]dto.ShowMenu, 0, len(menus))
	changed := false
	for i := range menus {
		menu := &menus[i]
		children, childChanged := removeSystemMenus(menu.Children, identity)
		if matchesSystemIdentity(*menu, identity) {
			filtered = append(filtered, children...)
			changed = true
			continue
		}
		if childChanged {
			menu.Children = children
			changed = true
		}
		filtered = append(filtered, *menu)
	}
	return filtered, changed
}

func matchesSystemIdentity(menu, canonical dto.ShowMenu) bool {
	return menu.ID == canonical.ID || menu.Label == canonical.Label || menu.Path == canonical.Path
}

func cloneMenus(menus []dto.ShowMenu) []dto.ShowMenu {
	if menus == nil {
		return nil
	}
	cloned := make([]dto.ShowMenu, len(menus))
	for i := range menus {
		cloned[i] = cloneMenu(menus[i])
	}
	return cloned
}

func cloneMenu(menu dto.ShowMenu) dto.ShowMenu {
	menu.Children = cloneMenus(menu.Children)
	return menu
}
