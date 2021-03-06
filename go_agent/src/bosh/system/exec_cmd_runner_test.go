package system_test

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	boshlog "bosh/logger"
	. "bosh/system"
)

func init() {
	Describe("execCmdRunner", func() {
		var (
			runner CmdRunner
		)

		BeforeEach(func() {
			runner = NewExecCmdRunner(boshlog.NewLogger(boshlog.LevelNone))
		})

		Describe("RunComplexCommand", func() {
			It("run complex command with working directory", func() {
				cmd := Command{
					Name:       "ls",
					Args:       []string{"-l"},
					WorkingDir: "../../..",
				}
				stdout, stderr, status, err := runner.RunComplexCommand(cmd)
				Expect(err).ToNot(HaveOccurred())
				Expect(stdout).To(ContainSubstring("README.md"))
				Expect(stdout).To(ContainSubstring("total"))
				Expect(stderr).To(BeEmpty())
				Expect(status).To(Equal(0))
			})

			It("run complex command with env", func() {
				cmd := Command{
					Name: "env",
					Env: map[string]string{
						"FOO": "BAR",
					},
				}
				stdout, stderr, status, err := runner.RunComplexCommand(cmd)
				Expect(err).ToNot(HaveOccurred())
				Expect(stdout).To(ContainSubstring("FOO=BAR"))
				Expect(stdout).To(ContainSubstring("PATH="))
				Expect(stderr).To(BeEmpty())
				Expect(status).To(Equal(0))
			})
		})

		Describe("RunComplexCommandAsync", func() {
			It("populates stdout and stderr", func() {
				cmd := Command{Name: "ls"}
				process, err := runner.RunComplexCommandAsync(cmd)
				Expect(err).ToNot(HaveOccurred())

				result := <-process.Wait()
				Expect(result.Error).ToNot(HaveOccurred())
				Expect(result.ExitStatus).To(Equal(0))
			})

			It("populates stdout and stderr", func() {
				cmd := Command{Name: "bash", Args: []string{"-c", "echo stdout >&1; echo stderr >&2"}}
				process, err := runner.RunComplexCommandAsync(cmd)
				Expect(err).ToNot(HaveOccurred())

				result := <-process.Wait()
				Expect(result.Error).ToNot(HaveOccurred())
				Expect(result.Stdout).To(Equal("stdout\n"))
				Expect(result.Stderr).To(Equal("stderr\n"))
			})

			It("returns error and sets status to exit status of comamnd if it command exits with non-0 status", func() {
				cmd := Command{Name: "bash", Args: []string{"-c", "exit 10"}}
				process, err := runner.RunComplexCommandAsync(cmd)
				Expect(err).ToNot(HaveOccurred())

				result := <-process.Wait()
				Expect(result.Error).To(HaveOccurred())
				Expect(result.ExitStatus).To(Equal(10))
			})

			It("allows setting custom env variable in addition to inheriting process env variables", func() {
				cmd := Command{
					Name: "env",
					Env: map[string]string{
						"FOO": "BAR",
					},
				}
				process, err := runner.RunComplexCommandAsync(cmd)
				Expect(err).ToNot(HaveOccurred())

				result := <-process.Wait()
				Expect(result.Error).ToNot(HaveOccurred())
				Expect(result.Stdout).To(ContainSubstring("FOO=BAR"))
				Expect(result.Stdout).To(ContainSubstring("PATH="))
			})

			It("changes working dir", func() {
				cmd := Command{Name: "bash", Args: []string{"-c", "echo $PWD"}, WorkingDir: "/tmp"}
				process, err := runner.RunComplexCommandAsync(cmd)
				Expect(err).ToNot(HaveOccurred())

				result := <-process.Wait()
				Expect(result.Error).ToNot(HaveOccurred())
				Expect(result.Stdout).To(ContainSubstring("/tmp"))
			})
		})

		Describe("RunCommand", func() {
			It("run command", func() {
				stdout, stderr, status, err := runner.RunCommand("echo", "Hello World!")
				Expect(err).ToNot(HaveOccurred())
				Expect(stdout).To(Equal("Hello World!\n"))
				Expect(stderr).To(BeEmpty())
				Expect(status).To(Equal(0))
			})

			It("run command with error output", func() {
				stdout, stderr, status, err := runner.RunCommand("bash", "-c", "echo error-output >&2")
				Expect(err).ToNot(HaveOccurred())
				Expect(stdout).To(BeEmpty())
				Expect(stderr).To(ContainSubstring("error-output"))
				Expect(status).To(Equal(0))
			})

			It("run command with non-0 exit status", func() {
				stdout, stderr, status, err := runner.RunCommand("bash", "-c", "exit 14")
				Expect(err).To(HaveOccurred())
				Expect(stdout).To(BeEmpty())
				Expect(stderr).To(BeEmpty())
				Expect(status).To(Equal(14))
			})

			It("run command with error", func() {
				stdout, stderr, status, err := runner.RunCommand("false")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("Running command: 'false', stdout: '', stderr: '': exit status 1"))
				Expect(stderr).To(BeEmpty())
				Expect(stdout).To(BeEmpty())
				Expect(status).To(Equal(1))
			})

			It("run command with error with args", func() {
				stdout, stderr, status, err := runner.RunCommand("false", "second arg")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("Running command: 'false second arg', stdout: '', stderr: '': exit status 1"))
				Expect(stderr).To(BeEmpty())
				Expect(stdout).To(BeEmpty())
				Expect(status).To(Equal(1))
			})

			It("run command with cmd not found", func() {
				stdout, stderr, status, err := runner.RunCommand("something that does not exist")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not found"))
				Expect(stderr).To(BeEmpty())
				Expect(stdout).To(BeEmpty())
				Expect(status).To(Equal(-1))
			})
		})

		Describe("CommandExists", func() {
			It("run command with input", func() {
				stdout, stderr, status, err := runner.RunCommandWithInput("foo\nbar\nbaz", "grep", "ba")
				Expect(err).ToNot(HaveOccurred())
				Expect(stdout).To(Equal("bar\nbaz\n"))
				Expect(stderr).To(BeEmpty())
				Expect(status).To(Equal(0))
			})
		})

		Describe("CommandExists", func() {
			It("command exists", func() {
				Expect(runner.CommandExists("env")).To(BeTrue())
				Expect(runner.CommandExists("absolutely-does-not-exist-ever-please-unicorns")).To(BeFalse())
			})
		})
	})

	Describe("execProcess", func() {
		var (
			runner CmdRunner
		)

		BeforeEach(func() {
			runner = NewExecCmdRunner(boshlog.NewLogger(boshlog.LevelNone))
		})

		Describe("TerminateNicely", func() {
			parentPidRe := regexp.MustCompile("parent_pid=\\d+")
			childPidRe := regexp.MustCompile("child_pid=\\d+")

			extractPid := func(output string, reg *regexp.Regexp) int {
				pidStr := reg.FindString(output)
				if pidStr == "" {
					panic(fmt.Sprintf("Failed to find pid in '%s' matching %v", output, reg))
				}

				pidStrParts := strings.SplitN(pidStr, "=", 2)
				if len(pidStrParts) != 2 {
					panic(fmt.Sprintf("Failed to extract pid from '%s'", pidStr))
				}

				pid, err := strconv.Atoi(pidStrParts[1])
				if err != nil {
					panic(fmt.Sprintf("Failed to convert pid '%s' to int: %s", pidStr, err.Error()))
				}

				return pid
			}

			expectProcessToNotExist := func(pid int) {
				process, err := os.FindProcess(pid)
				Expect(err).ToNot(HaveOccurred())
				Expect(process.Kill()).To(Equal(syscall.ESRCH)) // process not found
			}

			Context("when parent and child terminate after receiving SIGTERM", func() {
				It("sends term signal to the whole group and returns with exit status that parent exited", func() {
					cmd := Command{
						Name: "bash",
						Args: []string{"-c", `
exec 3>&1

function clean_up_parent {
	echo "Parent received SIGTERM"
	exit 13
}

function clean_up_child {
	echo "Child received SIGTERM" >&3
	exit 14
}

echo "parent_pid=$$"
trap clean_up_parent SIGTERM # Parent

echo $(
	echo "child_pid=$$" >&3
	trap clean_up_child SIGTERM # Child
	while true; do sleep 0.1; done
)
`},
					}
					process, err := runner.RunComplexCommandAsync(cmd)
					Expect(err).ToNot(HaveOccurred())

					// Wait for sh script to start and output pids
					time.Sleep(2 * time.Second)

					waitCh := process.Wait()

					err = process.TerminateNicely(1 * time.Minute)
					Expect(err).ToNot(HaveOccurred())

					result := <-waitCh
					Expect(result.Error).To(HaveOccurred())

					// Parent exit code is returned
					// bash adds 128 to signal status as exit code
					Expect(result.ExitStatus).To(Equal(13))

					// Term signal was sent to all processes in the group
					Expect(result.Stdout).To(ContainSubstring("Parent received SIGTERM"))
					Expect(result.Stdout).To(ContainSubstring("Child received SIGTERM"))

					// All processes are gone
					expectProcessToNotExist(extractPid(result.Stdout, parentPidRe))
					expectProcessToNotExist(extractPid(result.Stdout, childPidRe))
				})
			})

			Context("when parent and child do not exit after receiving SIGTERM in small amount of time", func() {
				It("sends kill signal to the whole group and returns with ? exit status", func() {
					cmd := Command{
						Name: "bash",
						Args: []string{"-c", `
exec 3>&1

function clean_up_noop { 'noop'; }

echo "parent_pid=$$"
trap clean_up_noop SIGTERM

echo $(
	echo "child_pid=$$" >&3
	trap clean_up_noop SIGTERM
	while true; do sleep 0.1; done
)
`},
					}
					process, err := runner.RunComplexCommandAsync(cmd)
					Expect(err).ToNot(HaveOccurred())

					// Wait for sh script to start and output pids
					time.Sleep(2 * time.Second)

					waitCh := process.Wait()

					err = process.TerminateNicely(2 * time.Second)
					Expect(err).ToNot(HaveOccurred())

					result := <-waitCh
					Expect(result.Error).To(HaveOccurred())

					// Parent exit code is returned
					Expect(result.ExitStatus).To(Equal(128 + 9))

					// Parent and child are killed
					expectProcessToNotExist(extractPid(result.Stdout, parentPidRe))
					expectProcessToNotExist(extractPid(result.Stdout, childPidRe))
				})
			})

			Context("when parent and child already exited before calling TerminateNicely", func() {
				It("returns without an error since all processes are gone", func() {
					cmd := Command{
						Name: "bash",
						Args: []string{"-c", `exit 0`},
					}
					process, err := runner.RunComplexCommandAsync(cmd)
					Expect(err).ToNot(HaveOccurred())

					// Wait for sh script to exit
					time.Sleep(2 * time.Second)

					waitCh := process.Wait()

					err = process.TerminateNicely(2 * time.Second)
					Expect(err).ToNot(HaveOccurred())

					result := <-waitCh
					Expect(result.Error).ToNot(HaveOccurred())
					Expect(result.Stdout).To(Equal(""))
					Expect(result.Stderr).To(Equal(""))
					Expect(result.ExitStatus).To(Equal(0))
				})
			})
		})
	})
}
