package app

import "hostward/internal/cli"

func Run(args []string) error {
	runner, err := cli.New()
	if err != nil {
		return err
	}

	return runner.Run(args)
}
