package ansible

import (
	"encoding/json"
	"errors"

	"github.com/cloud-lab-gateway/gateway/internal/domain/verify"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
)

type callbackOutput struct {
	Plays []callbackPlay                 `json:"plays"`
	Stats map[string]callbackStatsByHost `json:"stats"`
}

type callbackPlay struct {
	Tasks []callbackTask `json:"tasks"`
}

type callbackTask struct {
	Task  callbackTaskName              `json:"task"`
	Hosts map[string]callbackHostResult `json:"hosts"`
}

type callbackTaskName struct {
	Name string `json:"name"`
}

type callbackHostResult struct {
	Changed     bool        `json:"changed"`
	Failed      bool        `json:"failed"`
	Unreachable bool        `json:"unreachable"`
	Skipped     bool        `json:"skipped"`
	Msg         interface{} `json:"msg"`
	Stdout      interface{} `json:"stdout"`
	Stderr      interface{} `json:"stderr"`
}

type callbackStatsByHost struct {
	OK          int `json:"ok"`
	Changed     int `json:"changed"`
	Failures    int `json:"failures"`
	Failed      int `json:"failed"`
	Unreachable int `json:"unreachable"`
	Skipped     int `json:"skipped"`
}

func parseCallback(stdout []byte) (ports.CheckResult, error) {
	if len(stdout) == 0 {
		return ports.CheckResult{State: verify.StateErrored}, errors.New("ansible: empty json callback output")
	}

	var out callbackOutput
	if err := json.Unmarshal(stdout, &out); err != nil {
		return ports.CheckResult{State: verify.StateErrored, Stdout: string(stdout)}, err
	}

	result := ports.CheckResult{
		State:  verify.StatePassed,
		Stdout: string(stdout),
	}
	for _, play := range out.Plays {
		for _, task := range play.Tasks {
			hostResult, ok := firstHostResult(task.Hosts)
			if !ok {
				continue
			}
			step := verify.StepResult{
				TaskName: task.Task.Name,
				Status:   stepStatus(hostResult),
				Message:  resultMessage(hostResult),
			}
			if step.TaskName == "" {
				step.TaskName = "unnamed task"
			}
			if step.Status == verify.StepFailed || step.Status == verify.StepUnreachable {
				result.State = verify.StateFailed
			}
			result.Steps = append(result.Steps, step)
		}
	}

	result.Stats = sumStats(out.Stats)
	if result.Stats.Failed > 0 || result.Stats.Unreachable > 0 {
		result.State = verify.StateFailed
	}
	return result, nil
}

func firstHostResult(hosts map[string]callbackHostResult) (callbackHostResult, bool) {
	for _, result := range hosts {
		return result, true
	}
	return callbackHostResult{}, false
}

func stepStatus(result callbackHostResult) verify.StepStatus {
	switch {
	case result.Unreachable:
		return verify.StepUnreachable
	case result.Failed:
		return verify.StepFailed
	case result.Skipped:
		return verify.StepSkipped
	case result.Changed:
		return verify.StepChanged
	default:
		return verify.StepOK
	}
}

func resultMessage(result callbackHostResult) string {
	for _, value := range []interface{}{result.Msg, result.Stderr, result.Stdout} {
		switch v := value.(type) {
		case string:
			if v != "" {
				return v
			}
		case []interface{}, map[string]interface{}:
			if encoded, err := json.Marshal(v); err == nil {
				return string(encoded)
			}
		}
	}
	return ""
}

func sumStats(stats map[string]callbackStatsByHost) ports.AnsibleStats {
	var out ports.AnsibleStats
	for _, stat := range stats {
		out.OK += stat.OK
		out.Changed += stat.Changed
		out.Failed += stat.Failures + stat.Failed
		out.Unreachable += stat.Unreachable
		out.Skipped += stat.Skipped
	}
	return out
}
