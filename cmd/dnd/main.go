// Команда dnd — консольный прототип детективной игры. Ни одного вызова LLM.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/cli"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/rules/threshold"
)

func main() {
	casePath := flag.String("case", "cases/harbour/case.json", "путь к файлу дела")
	seed := flag.Int64("seed", 1, "seed RNG: прогон воспроизводится парой (seed, ввод)")
	script := flag.String("script", "", "файл команд вместо интерактивного ввода")
	flag.Parse()

	cfg, err := cases.Load(*casePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(*seed).Stream("resolve")

	in := os.Stdin
	if *script != "" {
		f, err := os.Open(*script)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer f.Close()
		in = f
	}

	if err := cli.NewSession(core.NewGame(*cfg), in, os.Stdout).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
