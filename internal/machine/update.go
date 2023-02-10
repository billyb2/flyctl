package machine

import (
	"context"
	"fmt"
	"time"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/watch"
	"github.com/superfly/flyctl/iostreams"
)

func Update(ctx context.Context, m *api.Machine, input *api.LaunchMachineInput) error {
	var (
		flapsClient = flaps.FromContext(ctx)
		io          = iostreams.FromContext(ctx)
		colorize    = io.ColorScheme()
	)

	if input != nil && input.Config != nil && input.Config.Guest != nil {
		// Check that there's a valid number of CPUs
		var validNumCpus []int

		if input.Config.Guest.CPUKind == "shared" {
			validNumCpus = append(validNumCpus, 1, 2, 4, 8)

		} else if input.Config.Guest.CPUKind == "performance" {
			validNumCpus = append(validNumCpus, 1, 2, 4, 8, 16, 64)

		}

		validCpuNum := false

		for _, num := range validNumCpus {
			if num == input.Config.Guest.CPUs {
				validCpuNum = true
				break

			}
		}

		if !validCpuNum {
			return fmt.Errorf("invalid config: invalid number of CPUs for %s guest. Valid numbers are %v", input.Config.Guest.CPUKind, validNumCpus)

		}

		// Check that the amount of memory is evenly divisible by 256 MiB
		if input.Config.Guest.MemoryMB%256 != 0 {
			return fmt.Errorf("invalid config: invalid memory size; must be in 256 MiB increment")

		}

		// Check that the amount of memory is valid
		var presetName string

		if input.Config.Guest.CPUKind == "shared" {
			presetName = fmt.Sprintf("shared-cpu-%dx", input.Config.Guest.CPUs)
		} else if input.Config.Guest.CPUKind == "performance" {
			presetName = fmt.Sprintf("performance-%dx", input.Config.Guest.CPUs)
		}

		// Check memory sizes
		if machinePreset, ok := api.MachinePresets[presetName]; ok {
			if machinePreset.MemoryMB > input.Config.Guest.MemoryMB {
				return fmt.Errorf("invalid config: for machines with %d CPUs, the minimum amount of memory is %d MiB", machinePreset.CPUs, machinePreset.MemoryMB)

			}

			var maxMemory int

			if input.Config.Guest.CPUKind == "shared" {
				maxMemory = input.Config.Guest.CPUs * api.MIN_MEMORY_MB_PER_SHARED_CPU
			} else if input.Config.Guest.CPUKind == "performance" {
				maxMemory = input.Config.Guest.CPUs * api.MIN_MEMORY_MB_PER_CPU
			}

			if input.Config.Guest.MemoryMB > maxMemory {
				return fmt.Errorf("invalid config: for machines with %d CPUs, the maximum amount of memory is %d MiB", machinePreset.CPUs, maxMemory)

			}

		}

	}

	fmt.Fprintf(io.Out, "Updating machine %s\n", colorize.Bold(m.ID))

	input.ID = m.ID
	if _, err := flapsClient.Update(ctx, *input, m.LeaseNonce); err != nil {
		return fmt.Errorf("could not stop machine %s: %w", input.ID, err)
	}

	waitForAction := "start"
	if m.Config.Schedule != "" {
		waitForAction = "stop"
	}

	if err := WaitForStartOrStop(ctx, &api.Machine{ID: input.ID}, waitForAction, time.Minute*5); err != nil {
		return err
	}

	if !input.SkipHealthChecks {
		if err := watch.MachinesChecks(ctx, []*api.Machine{m}); err != nil {
			return fmt.Errorf("failed to wait for health checks to pass: %w", err)
		}
	}

	fmt.Fprintf(io.Out, "Machine %s updated successfully!\n", colorize.Bold(m.ID))

	return nil
}
