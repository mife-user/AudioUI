package permission

import (
	"context"
	"errors"
	"path/filepath"
	"regexp"
	"strings"

	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

var (
	shellMetaPattern      = regexp.MustCompile("[|&;<>`$(){}]")
	destructivePattern    = regexp.MustCompile(`(?i)(\bformat\b|\bdiskpart\b|\bmkfs\b|\bshutdown\b|\breboot\b|\bbcdedit\b|\breg\s+delete\b|rm\s+-rf\s+[\\/]|rd\s+/s|del\s+/[fsq])`)
	bashUnsafePathPattern = regexp.MustCompile(`(?i)(~|%[a-z_]+%|\$home|[a-z]:|\\\\|(^|\s)/|\.\.|(^|[\s/])(runtime|artifacts)([\\/]|$))`)
	bashWriteFlagPattern  = regexp.MustCompile(`(?i)(--output|-fprintf|-fprint|-fls|-execdir|-exec|-ok|-delete|--ext-diff)`)
)

type pathAccess uint8

const (
	pathGeneric pathAccess = iota
	pathRead
	pathWrite
)

type Policy struct {
	Paths        port.PathResolver
	AllowSession func(key string) bool
}

func (p *Policy) Check(_ context.Context, tool string, input map[string]any) port.PolicyResult {
	rawName := strings.ToLower(strings.TrimSpace(tool))
	isMCP := strings.HasPrefix(rawName, "mcp__audioui__")
	name := strings.TrimPrefix(rawName, "mcp__audioui__")
	switch name {
	case "write", "edit", "notebookedit":
		paths, err := p.resolvePaths(input, pathWrite)
		if err != nil {
			return port.PolicyResult{Decision: port.PolicyDeny, Tier: valueobject.PermissionHardDeny, Reason: err.Error()}
		}
		if p.AllowSession != nil && len(paths) > 0 && p.AllowSession("write|"+paths[0]) {
			return port.PolicyResult{Decision: port.PolicyAllow, Tier: valueobject.PermissionAutoAllow, Reason: "本次运行已允许", Paths: paths}
		}
		return port.PolicyResult{
			Decision: port.PolicyConfirm,
			Tier:     valueobject.PermissionConfirm,
			Reason:   "文件写入需要确认",
			Risk:     "写文件",
			Paths:    paths,
		}
	case "read", "glob", "grep", "ls":
		if isMCP {
			if name != "grep" {
				return port.PolicyResult{Decision: port.PolicyConfirm, Tier: valueobject.PermissionConfirm, Reason: "未知 MCP 只读工具需要确认", Risk: "未知工具"}
			}
			return port.PolicyResult{
				Decision: port.PolicyAllow,
				Tier:     valueobject.PermissionAutoAllow,
				Reason:   "工作区叠加只读工具（服务端校验）",
				Paths:    inputPaths(input),
			}
		}
		paths, err := p.resolvePaths(input, pathRead)
		if err != nil {
			return port.PolicyResult{Decision: port.PolicyDeny, Tier: valueobject.PermissionHardDeny, Reason: err.Error(), Paths: paths}
		}
		return port.PolicyResult{Decision: port.PolicyAllow, Tier: valueobject.PermissionAutoAllow, Reason: "当前路径只读工具", Paths: paths}
	case "bash":
		return p.checkBash(input)
	case "reply", "ask_user", "get_chain", "get_graph", "get_node", "list_node_types", "validate", "read_artifact", "read_file", "list_dir", "stat":
		if name == "read_file" || name == "list_dir" || name == "stat" {
			// MCP's WorkspaceReader owns logical overlay validation. Resolving the
			// same logical path as a physical built-in path would reject valid
			// ancestor/runtime fallbacks before the server can inspect them.
			return port.PolicyResult{
				Decision: port.PolicyAllow,
				Tier:     valueobject.PermissionAutoAllow,
				Reason:   "工作区叠加只读工具（服务端校验）",
				Paths:    inputPaths(input),
			}
		}
		return port.PolicyResult{Decision: port.PolicyAllow, Tier: valueobject.PermissionAutoAllow, Reason: "应用内工具"}
	default:
		return port.PolicyResult{Decision: port.PolicyConfirm, Tier: valueobject.PermissionConfirm, Reason: "未知工具需要确认", Risk: "未知操作"}
	}
}

func (p *Policy) checkBash(input map[string]any) port.PolicyResult {
	command, _ := input["command"].(string)
	command = strings.TrimSpace(command)
	if command == "" {
		return port.PolicyResult{Decision: port.PolicyDeny, Tier: valueobject.PermissionHardDeny, Reason: "空命令"}
	}
	if destructivePattern.MatchString(command) || bashUnsafePathPattern.MatchString(command) {
		return port.PolicyResult{Decision: port.PolicyDeny, Tier: valueobject.PermissionHardDeny, Reason: "越界路径或破坏性命令", Command: command}
	}
	if p.AllowSession != nil && p.AllowSession("bash|"+command) {
		return port.PolicyResult{Decision: port.PolicyAllow, Tier: valueobject.PermissionAutoAllow, Reason: "本次运行已允许", Command: command}
	}
	if shellMetaPattern.MatchString(command) {
		return port.PolicyResult{Decision: port.PolicyConfirm, Tier: valueobject.PermissionConfirm, Reason: "包含 shell 元字符", Risk: "执行命令", Command: command}
	}
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return port.PolicyResult{Decision: port.PolicyDeny, Tier: valueobject.PermissionHardDeny, Reason: "空命令"}
	}
	if p.Paths != nil {
		for _, field := range fields[1:] {
			if strings.HasPrefix(field, "-") {
				continue
			}
			if _, err := p.Paths.Resolve(field); err != nil {
				return port.PolicyResult{Decision: port.PolicyDeny, Tier: valueobject.PermissionHardDeny, Reason: "Bash 参数路径越界", Command: command}
			}
		}
	}
	first := strings.ToLower(filepath.Base(fields[0]))
	switch first {
	case "pwd", "ls", "dir", "where", "cat", "type", "head", "tail", "grep", "rg":
		return port.PolicyResult{Decision: port.PolicyAllow, Tier: valueobject.PermissionAutoAllow, Reason: "只读命令", Command: command}
	case "find":
		if bashWriteFlagPattern.MatchString(command) {
			return port.PolicyResult{Decision: port.PolicyConfirm, Tier: valueobject.PermissionConfirm, Reason: "find 写入/执行选项", Risk: "执行命令", Command: command}
		}
		return port.PolicyResult{Decision: port.PolicyAllow, Tier: valueobject.PermissionAutoAllow, Reason: "只读 find", Command: command}
	case "git":
		if len(fields) > 1 && !strings.Contains(command, "--output") {
			switch fields[1] {
			case "status", "diff", "log", "show", "rev-parse":
				return port.PolicyResult{Decision: port.PolicyAllow, Tier: valueobject.PermissionAutoAllow, Reason: "只读 git", Command: command}
			}
		}
	}
	return port.PolicyResult{Decision: port.PolicyConfirm, Tier: valueobject.PermissionConfirm, Reason: "写命令/解释器/未知命令", Risk: "执行命令", Command: command}
}

func (p *Policy) resolvePaths(input map[string]any, access pathAccess) ([]string, error) {
	if p.Paths == nil {
		return nil, errors.New("PATH_DENIED: path resolver unavailable")
	}
	raw := inputPaths(input)
	paths := make([]string, 0, len(raw))
	for _, item := range raw {
		resolved, err := resolvePath(p.Paths, item, access)
		if err != nil {
			return []string{item}, err
		}
		paths = append(paths, resolved)
	}
	return paths, nil
}

func resolvePath(resolver port.PathResolver, path string, access pathAccess) (string, error) {
	if scoped, ok := resolver.(port.ScopedPathResolver); ok {
		switch access {
		case pathRead:
			return scoped.ResolveRead(path)
		case pathWrite:
			return scoped.ResolveWrite(path)
		}
	}
	return resolver.Resolve(path)
}

func inputPaths(input map[string]any) []string {
	keys := []string{"file_path", "path", "notebook_path"}
	var result []string
	for _, key := range keys {
		value, ok := input[key].(string)
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		result = append(result, value)
	}
	if len(result) == 0 {
		result = append(result, ".")
	}
	return result
}
