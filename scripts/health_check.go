package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"
)

type CheckResult struct {
	Name   string
	Status string // "READY", "WARNING", "FAILED"
	Info   string
}

func main() {
	fmt.Println("Perry Platform Pre-Flight Health Check")
	fmt.Println("======================================")

	checks := []func() CheckResult{
		checkGemini,
		checkAnthropic,
		checkDiscordTokens,
		checkLocalNode1,
		checkLocalNode2,
		checkConfigs,
		checkDocker,
		checkPythonTools,
	}

	failed := 0
	for _, check := range checks {
		res := check()
		symbol := "✅"
		if res.Status == "WARNING" {
			symbol = "⚠️"
		} else if res.Status == "FAILED" {
			symbol = "❌"
			failed++
		}
		fmt.Printf("%s [%-7s] %-20s: %s\n", symbol, res.Status, res.Name, res.Info)
	}

	fmt.Println("\nSummary:")
	if failed == 0 {
		fmt.Println("✨ All systems go. Mission is a GO.")
	} else {
		fmt.Printf("🛑 %d critical failures detected. Fix before proceeding.\n", failed)
	}
}

func checkGemini() CheckResult {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		return CheckResult{"Gemini API", "FAILED", "GEMINI_API_KEY not set"}
	}
	return CheckResult{"Gemini API", "READY", "Key present"}
}

func checkAnthropic() CheckResult {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return CheckResult{"Anthropic API", "WARNING", "Key not set (Anthropic is currently degraded)"}
	}
	return CheckResult{"Anthropic API", "READY", "Key present"}
}

func checkDiscordTokens() CheckResult {
	tokens := []string{
		"DISCORD_TOKEN_BOSS",
		"DISCORD_TOKEN_PHINEAS",
		"DISCORD_TOKEN_FERB",
		"DISCORD_TOKEN_CARL",
		"DISCORD_TOKEN_DOOF",
	}
	envMissing := 0
	for _, t := range tokens {
		if os.Getenv(t) == "" {
			envMissing++
		}
	}

	yamlPresent := false
	if _, err := os.Stat("configs/discord.yaml"); err == nil {
		yamlPresent = true
	}

	if envMissing == 0 {
		return CheckResult{"Discord Tokens", "READY", "All tokens present in environment"}
	}
	if yamlPresent {
		return CheckResult{"Discord Tokens", "READY", "Tokens present in configs/discord.yaml"}
	}

	return CheckResult{"Discord Tokens", "FAILED", "No tokens found in environment or configs/discord.yaml"}
}

func checkLocalNode1() CheckResult {
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://10.3.2.8:11434/api/tags")
	if err != nil {
		return CheckResult{"Node 1 (diffuser)", "FAILED", "Unreachable (10.3.2.8)"}
	}
	defer resp.Body.Close()
	return CheckResult{"Node 1 (diffuser)", "READY", "Responding on 11434"}
}

func checkLocalNode2() CheckResult {
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://10.3.2.48:11434/api/tags")
	if err != nil {
		return CheckResult{"Node 2 (mink)", "FAILED", "Unreachable (10.3.2.48)"}
	}
	defer resp.Body.Close()
	return CheckResult{"Node 2 (mink)", "READY", "Responding on 11434"}
}

func checkConfigs() CheckResult {
	files := []string{"configs/routing.yaml", "configs/policy.yaml", "configs/discord.yaml"}
	for _, f := range files {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			return CheckResult{"Config Files", "FAILED", fmt.Sprintf("Missing %s", f)}
		}
	}
	if _, err := os.Stat(".perry/perry.db"); os.IsNotExist(err) {
		return CheckResult{"Database", "WARNING", "perry.db not found (will be created on start)"}
	}
	return CheckResult{"Config Files", "READY", "All core configs present"}
}

func checkDocker() CheckResult {
	_, err := exec.LookPath("docker")
	if err != nil {
		return CheckResult{"Docker", "WARNING", "Binary not found in PATH"}
	}
	err = exec.Command("docker", "ps").Run()
	if err != nil {
		return CheckResult{"Docker", "FAILED", "Daemon not responding"}
	}
	return CheckResult{"Docker", "READY", "Daemon active"}
}

func checkPythonTools() CheckResult {
	files := []string{"python/audit/ast_analyzer.py", "python/audit/secrets_scanner.py"}
	for _, f := range files {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			return CheckResult{"Python Tools", "FAILED", fmt.Sprintf("Missing %s", f)}
		}
	}
	return CheckResult{"Python Tools", "READY", "Audit scripts present"}
}
