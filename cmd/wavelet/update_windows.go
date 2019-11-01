package main

import "github.com/pkg/errors"
import "os"

func switchToUpdatedBinary(newBinary string, atStartup bool) error {
	var attrs os.ProcAttr

	/*
	 * Windows does not provide a convienent way to replace the
	 * process image, so we can really only switch to the new
	 * binary before we have started doing any work as BOTH
	 * processes will be running simultaneously.  The "atStartup"
	 * flag indicates whether the switch has been called at start-up
	 * prior to any goroutines/threads being created such that it
	 * is safe to execute the child process and wait for it to exit.
	 */
	if !atStartup {
		/*
		 * XXX:TODO: It might be a safer idea to call os.Exit()
		 * here to cause the user or service manager to restart
		 * with the updated binary rather than letting them run
		 * the old version with no notification
		 */
		return errors.New("Unable to switch to new upgraded binary except as at startup on Windows")
	}

	/*
	 * Start the new child process, passing in the standard
	 * file descriptors
	 *
	 * We currently do not bother to mutate os.Args[0] to
	 * the new executable pathname since it is not examined
	 * but we should consider it in the future
	 */
	attrs.Files = []*os.File{os.Stdin, os.Stdout, os.Stderr}
	process, err := os.StartProcess(newBinary, os.Args, &attrs)
	if err != nil {
		return errors.Wrap(err, "Unable to execute new process during auto-update")
	}

	/*
	 * Return value to pass along from the child process
	 */
	returnValue := 0

	/*
	 * Wait for the child process to terminate
	 */
	processStatus, err := process.Wait()
	if err == nil {
		if !processStatus.Success() {
			returnValue = processStatus.ExitCode()
		}
	} else {
		returnValue = 100
	}

	/*
	 * Pass along any exit code from the child process
	 * to our caller
	 */
	os.Exit(returnValue)

	/*
	 * This should never be reached as "os.Exit()" never
	 * returns
	 */
	return errors.New("Internal error")
}
