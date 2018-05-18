package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/render"
	"github.com/goji/httpauth"
	"gopkg.in/src-d/go-git.v4"
)

type Config struct {
	Password     string `json:"password"`
	StartScript  string `json:"start_script"`
	StopScript   string `json:"stop_script"`
	UpdateScript string `json:"update_script"`
	GitDir       string `json:"git_dir"`
	PidFile      string `json:"pid_file"`
}

type Response struct {
	Success bool        `json:"success"`
	Message interface{} `json:"message"`
}

func (resp *Response) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}

func NewResponse(success bool, message interface{}) render.Renderer {
	return &Response{
		Success: success,
		Message: message,
	}
}

type Monitor struct {
	Conf *Config
}

func (m *Monitor) Start(w http.ResponseWriter, r *http.Request) {
	cmd := exec.Command(m.Conf.StartScript)

	success := true
	message := "Server has been started"
	if err := cmd.Run(); err != nil {
		success = false
		message = fmt.Sprintf("Server failed to start (%v)", err)
	}

	render.Render(w, r, NewResponse(success, message))
}

func (m *Monitor) Stop(w http.ResponseWriter, r *http.Request) {
	cmd := exec.Command(m.Conf.StopScript)

	success := true
	message := "Server has been stopped"
	if err := cmd.Run(); err != nil {
		success = false
		message = fmt.Sprintf("Server failed to stop (%v)", err)
	}

	render.Render(w, r, NewResponse(success, message))
}

func (m *Monitor) Update(w http.ResponseWriter, r *http.Request) {
	cmd := exec.Command(m.Conf.UpdateScript)

	success := true
	message := "Server has been updated"
	if err := cmd.Run(); err != nil {
		success = false
		message = fmt.Sprintf("Server failed to update (%v)", err)
	}

	render.Render(w, r, NewResponse(success, message))
}

func (m *Monitor) Commit(w http.ResponseWriter, r *http.Request) {
	success, message := func() (bool, interface{}) {
		repo, err := git.PlainOpen(m.Conf.GitDir)
		if err != nil {
			return false, fmt.Sprintf("Failed to open git repo (%v)", err)
		}

		head, err := repo.Head()
		if err != nil {
			return false, fmt.Sprintf("Failed to get HEAD ref (%v)", err)
		}

		commit, err := repo.CommitObject(head.Hash())
		if err != nil {
			return false, fmt.Sprintf("Failed to get HEAD commit (%v)", err)
		}

		summary := strings.SplitN(strings.TrimSpace(commit.Message), "\n", 2)[0]

		return true, map[string]string{
			"message": summary,
			"date":    commit.Committer.When.Format("Mon Jan 02 15:04:05 2006 -0700"),
			"sha":     commit.Hash.String(),
		}
	}()

	render.Render(w, r, NewResponse(success, message))
}

func (m *Monitor) IsRunning(w http.ResponseWriter, r *http.Request) {
	running := true

	bpid, err := ioutil.ReadFile(m.Conf.PidFile)
	pid, perr := strconv.Atoi(strings.TrimSpace(string(bpid)))
	if err != nil || perr != nil || syscall.Kill(pid, syscall.Signal(0)) != nil {
		running = false
	}

	render.Render(w, r, NewResponse(true, running))
}

func main() {
	file, err := ioutil.ReadFile("config.json")
	if err != nil {
		fmt.Printf("config read error: %v\n", err)
		os.Exit(1)
	}

	var config Config
	if err = json.Unmarshal(file, &config); err != nil {
		fmt.Printf("config parse error: %v\n", err)
		os.Exit(1)
	}

	monitor := &Monitor{&config}

	fmt.Print("starting\n")

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(render.SetContentType(render.ContentTypeJSON))
	r.Use(httpauth.SimpleBasicAuth("auth", config.Password))

	r.Post("/start", monitor.Start)
	r.Post("/stop", monitor.Stop)
	r.Post("/update", monitor.Update)
	r.Get("/commit", monitor.Commit)
	r.Get("/is_running", monitor.IsRunning)

	http.ListenAndServe(":3889", r)
}