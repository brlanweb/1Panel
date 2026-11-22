package service

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/buserr"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	alertUtil "github.com/1Panel-dev/1Panel/agent/utils/alert"
	"github.com/1Panel-dev/1Panel/agent/utils/encrypt"
	"gorm.io/gorm"
)

// Cluster alert state machine.
//
// Every rule watches one or more targets (a node metric, a disk, a URL, a
// model endpoint). Each target has its own row in AlertTargetState:
//
//   - a failed check increments ConsecutiveFail; once it reaches the rule's
//     Count the target is failing and notifications start;
//   - at most SendCount notifications go out per failure, one per check
//     cycle at most; after the last one the target is silenced for
//     CooldownHours, then the next SendCount round may start;
//   - a successful check after an alert sends one recovery notice and resets
//     every counter and the cooldown.
//
// Failures and recoveries found in the same check are merged into one
// message per rule.

const (
	clusterTargetOK   = "ok"
	clusterTargetFail = "fail"

	clusterDefaultCooldownHours = 24
	clusterMaxCooldownHours     = 720
	clusterMaxCycleMinutes      = 1440
	clusterMaxTimeoutSec        = 300
	clusterMaxDetailTargets     = 20
)

// Channels allowed for the cluster alert types.
var clusterAlertMethodTypes = map[string]bool{
	constant.Email:    true,
	constant.WeCom:    true,
	constant.Telegram: true,
	constant.Bark:     true,
	constant.Custom:   true,
}

var clusterStateMu sync.Mutex

// clusterSendAllowed is the send time window check (replaceable in tests).
var clusterSendAllowed = alertUtil.CheckSendTimeRange

func parseClusterAlertParams(raw string) dto.ClusterAlertParams {
	var params dto.ClusterAlertParams
	if strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &params)
	}
	return params
}

func clusterCooldown(params dto.ClusterAlertParams) time.Duration {
	hours := params.CooldownHours
	if hours <= 0 {
		hours = clusterDefaultCooldownHours
	}
	return time.Duration(hours) * time.Hour
}

// maskClusterAlertParams hides the stored API key from API responses.
func maskClusterAlertParams(alertType, raw string) string {
	if alertType != dto.ClusterAlertModel {
		return raw
	}
	params := parseClusterAlertParams(raw)
	params.APIKeySet = params.APIKey != ""
	params.APIKey = ""
	data, err := json.Marshal(params)
	if err != nil {
		return ""
	}
	return string(data)
}

// prepareClusterAlert validates and normalises a cluster rule before it is
// stored. previousParams is the stored AdvancedParams of the rule being
// updated (empty on create); an empty API key keeps the stored one.
func prepareClusterAlert(alertType string, cycle, count, sendCount *uint, method, advanced, previousParams string) (string, error) {
	if err := validateClusterAlertMethods(method); err != nil {
		return "", err
	}
	if *cycle == 0 {
		*cycle = 1
	}
	if *cycle > clusterMaxCycleMinutes {
		return "", clusterParamErr("cycle must be between 1 and 1440 minutes")
	}
	if *count == 0 {
		*count = 1
	}
	if *count > 100 {
		return "", clusterParamErr("failure threshold must be between 1 and 100")
	}
	if *sendCount == 0 {
		*sendCount = 1
	}
	if *sendCount > 100 {
		return "", clusterParamErr("notification count must be between 1 and 100")
	}
	params := parseClusterAlertParams(advanced)
	if params.CooldownHours <= 0 {
		params.CooldownHours = clusterDefaultCooldownHours
	}
	if params.CooldownHours > clusterMaxCooldownHours {
		return "", clusterParamErr("cooldown must be between 1 and 720 hours")
	}
	checkTimeout := func(defaultSec int) error {
		if params.TimeoutSec <= 0 {
			params.TimeoutSec = defaultSec
		}
		if params.TimeoutSec > clusterMaxTimeoutSec {
			return clusterParamErr("timeout must be between 1 and 300 seconds")
		}
		if params.TimeoutSec >= int(*cycle)*60 {
			return clusterParamErr("timeout must be shorter than the check interval")
		}
		return nil
	}
	switch alertType {
	case dto.ClusterAlertResource:
		for _, value := range []float64{params.CPU, params.Memory, params.Disk} {
			if value < 0 || value > 100 {
				return "", clusterParamErr("thresholds must be between 0 and 100")
			}
		}
		if params.CPU == 0 && params.Memory == 0 && params.Disk == 0 && !params.Offline {
			return "", clusterParamErr("enable at least one metric")
		}
		cleaned := make([]string, 0, len(params.DiskExclude))
		for _, item := range params.DiskExclude {
			if item = strings.TrimSpace(item); item != "" {
				cleaned = append(cleaned, item)
			}
		}
		params.DiskExclude = cleaned
		// Probe-only fields do not belong to this type.
		params.URL, params.Provider, params.APIKey, params.TimeoutSec, params.UseProxy = "", "", "", 0, false
	case dto.ClusterAlertSite:
		parsed, err := url.Parse(strings.TrimSpace(params.URL))
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return "", clusterParamErr("URL must be an http(s) address")
		}
		params.URL = strings.TrimSpace(params.URL)
		params.Method = strings.ToUpper(strings.TrimSpace(params.Method))
		if params.Method == "" {
			params.Method = "GET"
		}
		if params.Method != "GET" && params.Method != "HEAD" && params.Method != "POST" {
			return "", clusterParamErr("method must be GET, HEAD or POST")
		}
		if _, err := parseExpectStatus(params.ExpectStatus); err != nil {
			return "", err
		}
		if err := checkTimeout(10); err != nil {
			return "", err
		}
	case dto.ClusterAlertModel:
		switch params.Provider {
		case "openai", "anthropic", "gemini":
		default:
			return "", clusterParamErr("provider must be openai, anthropic or gemini")
		}
		params.Model = strings.TrimSpace(params.Model)
		if params.Model == "" {
			return "", clusterParamErr("model is required")
		}
		params.BaseURL = strings.TrimSpace(params.BaseURL)
		if params.BaseURL != "" {
			parsed, err := url.Parse(params.BaseURL)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
				return "", clusterParamErr("base URL must be an http(s) address")
			}
		}
		if params.MaxTokens <= 0 || params.MaxTokens > 1024 {
			params.MaxTokens = 16
		}
		if err := checkTimeout(30); err != nil {
			return "", err
		}
		key := strings.TrimSpace(params.APIKey)
		if key == "" {
			key = parseClusterAlertParams(previousParams).APIKey
			if key == "" {
				return "", clusterParamErr("API key is required")
			}
		} else {
			encrypted, err := encrypt.StringEncrypt(key)
			if err != nil {
				return "", err
			}
			key = encrypted
		}
		params.APIKey = key
	}
	params.APIKeySet = false
	data, err := json.Marshal(params)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func clusterParamErr(detail string) error {
	return buserr.WithDetail("ErrInvalidParams", detail, nil)
}

func validateClusterAlertMethods(method string) error {
	found := false
	for _, item := range strings.Split(method, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		configType := ""
		if id, err := strconv.ParseUint(item, 10, 64); err == nil {
			config, err := alertRepo.GetConfigById(uint(id))
			if err != nil {
				return err
			}
			configType = config.Type
		} else {
			configType = legacyAlertMethodTypeMap[item]
		}
		if !clusterAlertMethodTypes[configType] {
			return buserr.WithErr("ErrAlertMethodNotSupported", nil)
		}
		found = true
	}
	if !found {
		return buserr.WithErr("ErrAlertMethodNotSupported", nil)
	}
	return nil
}

func clearClusterTargetStates(alertID uint) {
	if global.AlertDB == nil {
		return
	}
	clusterStateMu.Lock()
	defer clusterStateMu.Unlock()
	if err := global.AlertDB.Where("alert_id = ?", alertID).Delete(&model.AlertTargetState{}).Error; err != nil {
		global.LOG.Errorf("clear cluster alert states of %d failed: %v", alertID, err)
	}
}

// ---------------------------------------------------------------- engine

type clusterTransition struct {
	label  string
	value  string
	errMsg string
}

type clusterDecision struct {
	alerts     []clusterTransition
	recoveries []clusterTransition
}

// applyClusterCheck runs the state machine for one rule and returns the
// transitions that need a notification. It persists the new states.
func applyClusterCheck(db *gorm.DB, alert dto.AlertDTO, targets []dto.ClusterAlertTargetResult, now time.Time) (clusterDecision, error) {
	clusterStateMu.Lock()
	defer clusterStateMu.Unlock()

	var decision clusterDecision
	if len(targets) == 0 {
		return decision, nil
	}
	var existing []model.AlertTargetState
	if err := db.Where("alert_id = ?", alert.ID).Find(&existing).Error; err != nil {
		return decision, err
	}
	states := make(map[string]model.AlertTargetState, len(existing))
	for _, item := range existing {
		states[item.TargetKey] = item
	}
	params := parseClusterAlertParams(alert.AdvancedParams)
	threshold := alert.Count
	if threshold == 0 {
		threshold = 1
	}
	sendLimit := alert.SendCount
	if sendLimit == 0 {
		sendLimit = 1
	}
	cycle := alert.Cycle
	if cycle == 0 {
		cycle = 1
	}
	// Leave a little slack so a check that runs a few seconds early still
	// counts as the next cycle.
	minGap := time.Duration(cycle)*time.Minute - 10*time.Second
	inTimeRange := clusterSendAllowed(alert.Type)
	nowUnix := now.Unix()

	for _, target := range targets {
		if strings.TrimSpace(target.Key) == "" {
			continue
		}
		state, ok := states[target.Key]
		if !ok {
			state = model.AlertTargetState{AlertID: alert.ID, TargetKey: target.Key, Status: clusterTargetOK, LastChangeAt: nowUnix}
		}
		state.TargetLabel = truncateRunes(firstNonBlank(target.Label, target.Key), 250)
		state.LastCheckAt = nowUnix
		state.LastValue = truncateRunes(target.Value, 60)
		transition := clusterTransition{label: state.TargetLabel, value: target.Value, errMsg: target.Error}

		if target.Failed {
			state.ConsecutiveFail++
			state.LastError = truncateRunes(target.Error, 500)
			if state.ConsecutiveFail >= threshold {
				if state.Status != clusterTargetFail {
					state.Status = clusterTargetFail
					state.LastChangeAt = nowUnix
				}
				if state.CooldownUntil > 0 && nowUnix >= state.CooldownUntil {
					state.CooldownUntil = 0
					state.SentCount = 0
				}
				due := state.LastAlertAt == 0 || now.Sub(time.Unix(state.LastAlertAt, 0)) >= minGap
				if state.CooldownUntil == 0 && state.SentCount < sendLimit && due && inTimeRange {
					decision.alerts = append(decision.alerts, transition)
					state.SentCount++
					state.Alerted = true
					state.LastAlertAt = nowUnix
					if state.SentCount >= sendLimit {
						state.CooldownUntil = now.Add(clusterCooldown(params)).Unix()
					}
				}
			}
		} else {
			if state.Status == clusterTargetFail && state.Alerted && inTimeRange {
				decision.recoveries = append(decision.recoveries, transition)
			}
			if state.Status != clusterTargetOK {
				state.LastChangeAt = nowUnix
			}
			state.Status = clusterTargetOK
			state.ConsecutiveFail = 0
			state.SentCount = 0
			state.Alerted = false
			state.CooldownUntil = 0
			state.LastAlertAt = 0
			state.LastError = ""
		}
		if err := db.Save(&state).Error; err != nil {
			return decision, err
		}
	}
	return decision, nil
}

// ProcessClusterChecks evaluates check results and sends notifications.
func ProcessClusterChecks(checks []dto.ClusterAlertCheck) error {
	if len(checks) == 0 {
		return nil
	}
	alerts, err := NewIAlertService().GetAlerts()
	if err != nil {
		return err
	}
	byID := make(map[uint]dto.AlertDTO, len(alerts))
	for _, item := range alerts {
		if dto.IsClusterAlertType(item.Type) {
			byID[item.ID] = item
		}
	}
	for _, check := range checks {
		alert, ok := byID[check.AlertID]
		if !ok {
			continue
		}
		processClusterCheck(alert, check.Targets)
	}
	return nil
}

func processClusterCheck(alert dto.AlertDTO, targets []dto.ClusterAlertTargetResult) {
	decision, err := applyClusterCheck(global.AlertDB, alert, targets, time.Now())
	if err != nil {
		global.LOG.Errorf("cluster alert %d state update failed: %v", alert.ID, err)
		return
	}
	sendClusterNotification(alert, "fail", decision.alerts)
	sendClusterNotification(alert, "recover", decision.recoveries)
}

func sendClusterNotification(alert dto.AlertDTO, kind string, items []clusterTransition) {
	if len(items) == 0 {
		return
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].label < items[j].label })
	lines := make([]string, 0, len(items))
	for i, item := range items {
		if i >= clusterMaxDetailTargets {
			lines = append(lines, fmt.Sprintf("… +%d", len(items)-clusterMaxDetailTargets))
			break
		}
		line := item.label
		if kind == "fail" {
			detail := firstNonBlank(item.errMsg, item.value)
			if detail != "" {
				line += " (" + truncateRunes(detail, 160) + ")"
			}
		} else if item.value != "" {
			line += " (" + truncateRunes(item.value, 60) + ")"
		}
		lines = append(lines, line)
	}
	params := []dto.Param{
		{Index: "1", Key: "kind", Value: kind},
		{Index: "2", Key: "title", Value: alert.Title},
		{Index: "3", Key: "detail", Value: strings.Join(lines, "; ")},
		{Index: "4", Key: "count", Value: strconv.Itoa(len(items))},
	}
	// Never let the stored (encrypted) API key reach logs or webhooks.
	alert.AdvancedParams = maskClusterAlertParams(alert.Type, alert.AdvancedParams)
	sender := NewAlertSender(alert, "cluster-"+kind)
	sender.skipDailyQuota = true
	sender.ResourceSend(strconv.Itoa(len(items)), params)
}

// ListClusterTargetStates returns the per-target states of one rule.
func ListClusterTargetStates(alertID uint) ([]dto.ClusterAlertTargetStateDTO, error) {
	var rows []model.AlertTargetState
	if err := global.AlertDB.Where("alert_id = ?", alertID).Order("target_label asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	res := make([]dto.ClusterAlertTargetStateDTO, 0, len(rows))
	for _, row := range rows {
		res = append(res, dto.ClusterAlertTargetStateDTO{
			TargetKey:       row.TargetKey,
			TargetLabel:     row.TargetLabel,
			Status:          row.Status,
			ConsecutiveFail: row.ConsecutiveFail,
			SentCount:       row.SentCount,
			CooldownUntil:   row.CooldownUntil,
			LastCheckAt:     row.LastCheckAt,
			LastChangeAt:    row.LastChangeAt,
			LastValue:       row.LastValue,
			LastError:       row.LastError,
		})
	}
	return res, nil
}

// ListClusterResourceRules returns the enabled cluster resource rules for
// the core sampler.
func ListClusterResourceRules() ([]dto.ClusterAlertRule, error) {
	alerts, err := alertRepo.List(repo.WithByStatus(constant.AlertEnable), alertRepo.WithByType(dto.ClusterAlertResource))
	if err != nil {
		return nil, err
	}
	res := make([]dto.ClusterAlertRule, 0, len(alerts))
	for _, item := range alerts {
		res = append(res, dto.ClusterAlertRule{
			ID:        item.ID,
			Type:      item.Type,
			Title:     item.Title,
			Cycle:     item.Cycle,
			Count:     item.Count,
			SendCount: item.SendCount,
			Params:    parseClusterAlertParams(item.AdvancedParams),
		})
	}
	return res, nil
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "…"
}
