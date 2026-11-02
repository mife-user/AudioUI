package app

import "fmt"

type runMode int

const (
	modeGUI runMode = iota
	modeVersion
	modeHelp
	modeSelfCheck
)

func parseMode(args []string) runMode {
	for _, arg := range args {
		switch arg {
		case "--version", "-version":
			return modeVersion
		case "--help", "-h", "-help":
			return modeHelp
		case "--self-check":
			return modeSelfCheck
		}
	}
	return modeGUI
}

func printHelp() {
	fmt.Println("AudioUI")
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  audio-ui.exe              启动工作台")
	fmt.Println("  audio-ui.exe --version    输出版本")
	fmt.Println("  audio-ui.exe --self-check 无界面环境自检并写入 logs/self-check.json")
	fmt.Println()
	fmt.Println("环境变量:")
	fmt.Println("  AUDIOUI_HOME          覆盖便携根目录（开发/诊断）")
	fmt.Println("  AUDIOUI_LOG_STDERR=1  同时把日志写到 stderr")
}
