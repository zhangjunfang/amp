package local

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

const (
	InitTimeout       = 10
	ContainerName     = "ampagent"
	ImageName         = "appcelerator/ampagent"
	DockerSocket      = "/var/run/docker.sock"
	DockerSwarmSocket = "/var/run/docker"
)

var (
	ContainerLabels = map[string]string{"io.amp.role": "infrastructure"}
)

// RequestOptions stores parameters for the Docker API
type RequestOptions struct {
	InitRequest swarm.InitRequest
	// Node labels
	Labels map[string]string
	// Tag of the ampagent image
	Tag           string
	Registration  string
	Notifications bool
	ForceLeave    bool
	SkipTests     bool
	NoMonitoring  bool
}

type FullSwarmInfo struct {
	Swarm swarm.Swarm
	Node  swarm.Node
}
type ShortSwarmInfo struct {
	SwarmID string `json:"SwarmID"`
	NodeID  string `json:"NodeID"`
}

// EnsureSwarmExists checks that the Swarm is initialized, and does it if it's not the case
func EnsureSwarmExists(ctx context.Context, c *client.Client, opts *RequestOptions) error {
	timeout := make(chan bool, 1)
	done := make(chan bool, 1)
	var err error
	// the Init method may freeze if the Docker engine has issues (and it often has)
	go func() {
		time.Sleep(InitTimeout * time.Second)
		timeout <- true
	}()
	go func() {
		_, err = c.SwarmInit(ctx, opts.InitRequest)
		if err != nil {
			// if the swarm is already initialized, ignore the error
			if strings.Contains(fmt.Sprintf("%v", err), "This node is already part of a swarm") {
				err = nil
			} else {
				fmt.Printf("%v\n", err)
			}
		}
		done <- true
	}()
	select {
	case <-done:
		return err
	case <-timeout:
		return fmt.Errorf("Timed out")
	}
}

func LabelNode(ctx context.Context, c *client.Client, opts *RequestOptions) error {
	node, err := InfoNode(ctx, c)
	if err != nil {
		return err
	}
	version := node.Meta.Version
	nodeSpec := node.Spec
	for label := range opts.Labels {
		node.Spec.Annotations.Labels[label] = opts.Labels[label]
	}
	return c.NodeUpdate(ctx, node.ID, version, nodeSpec)
}

func removeAgent(ctx context.Context, c *client.Client, cid string) error {
	return c.ContainerRemove(ctx, cid, types.ContainerRemoveOptions{Force: true})
}

// RunAgent runs the ampagent image to init (action ="install") or destroy (action="uninstall")
func RunAgent(ctx context.Context, c *client.Client, action string, opts *RequestOptions) error {
	containerName := ContainerName
	image := fmt.Sprintf("%s:%s", ImageName, opts.Tag)
	config := container.Config{
		Image: image,
		Env: []string{
			fmt.Sprintf("TAG=%s", opts.Tag),
			fmt.Sprintf("REGISTRATION=%s", opts.Registration),
			fmt.Sprintf("NOTIFICATIONS=%t", opts.Notifications),
		},
		Labels: ContainerLabels,
		Tty:    false,
	}
	var actionArgs []string
	if opts.SkipTests {
		actionArgs = append(actionArgs, "--fast")
	}
	if opts.NoMonitoring {
		actionArgs = append(actionArgs, "--no-monitoring")
	}
	switch action {
	case "install":
		action = ""
		config.Cmd = actionArgs
	case "uninstall":
		config.Cmd = append([]string{action}, actionArgs...)
	default:
		return fmt.Errorf("action %s is not implemented", action)
	}
	mounts := []mount.Mount{
		mount.Mount{
			Type:   "bind",
			Source: DockerSocket,
			Target: DockerSocket,
		},
		mount.Mount{
			Type:   "bind",
			Source: DockerSwarmSocket,
			Target: DockerSwarmSocket,
		},
	}
	hostConfig := container.HostConfig{
		AutoRemove: false,
		Mounts:     mounts,
	}
	reader, err := c.ImagePull(ctx, image, types.ImagePullOptions{})
	if err != nil {
		// don't exit, if not in the registry we may still want to run the container with a local image
		fmt.Println("ampagent image pull failed, which is expected on a development version")
	} else {
		// wait for the image to be pulled
		data := make([]byte, 1000, 1000)
		for {
			_, err := reader.Read(data)
			if err != nil {
				if err.Error() == "EOF" {
					break
				}
				return err
			}
		}
	}
	r, err := c.ContainerCreate(ctx, &config, &hostConfig, nil, containerName)
	if err != nil {
		return err
	}

	done := make(chan bool, 1)
	interruption := make(chan os.Signal, 1)
	signal.Notify(interruption, os.Interrupt, os.Kill)
	go func() {
		sig := <-interruption
		fmt.Printf("Received signal %s\n", sig.String())
		err = c.ContainerKill(ctx, r.ID, "SIGINT")
		if err != nil {
			fmt.Printf("%v\n", err)
		}
		_ = removeAgent(ctx, c, r.ID)
		done <- true
		return
	}()

	go func() {
		var timestamp int64
		for {
			reader, err := c.ContainerLogs(ctx, r.ID, types.ContainerLogsOptions{
				ShowStdout: true,
				ShowStderr: true,
				Follow:     false,
				Since:      strconv.FormatInt(timestamp, 10),
			})
			timestamp = time.Now().Unix()
			if err != nil {
				return
			}
			defer reader.Close()
			_, err = stdcopy.StdCopy(os.Stdout, os.Stderr, reader)
			time.Sleep(time.Second)
		}
	}()
	if err = c.ContainerStart(ctx, r.ID, types.ContainerStartOptions{}); err != nil {
		_ = removeAgent(ctx, c, r.ID)
		return err
	}

	go func() {
		for {
			filter := filters.NewArgs()
			filter.Add("id", r.ID)
			l, _ := c.ContainerList(ctx, types.ContainerListOptions{Filters: filter})
			if len(l) == 0 {
				done <- true
				return
			}
			time.Sleep(time.Second)
		}
	}()

	<-done
	// give time to clear the logs
	time.Sleep(1200 * time.Millisecond)
	_ = removeAgent(ctx, c, r.ID)
	return nil
}

// InfoCluster returns the Swarm info
func InfoCluster(ctx context.Context, c *client.Client) (swarm.Swarm, error) {
	return c.SwarmInspect(ctx)
}

// InfoNode returns the Node info
func InfoNode(ctx context.Context, c *client.Client) (swarm.Node, error) {
	nodes, err := c.NodeList(ctx, types.NodeListOptions{})
	if len(nodes) != 1 {
		return swarm.Node{}, fmt.Errorf("expected 1 node, got %d", len(nodes))
	}
	node, _, err := c.NodeInspectWithRaw(ctx, nodes[0].ID)
	return node, err
}

func InfoToJSON(sw swarm.Swarm, node swarm.Node) (string, error) {
	// filter the swarm content
	si := ShortSwarmInfo{SwarmID: sw.ID, NodeID: node.ID}
	j, err := json.Marshal(si)
	if err != nil {
		return "", err
	}
	return string(j), nil
}

// DeleteSwarm starts the delete operation
func DeleteCluster(ctx context.Context, c *client.Client, opts *RequestOptions) error {
	if opts.ForceLeave {
		return c.SwarmLeave(ctx, true)
	}
	return nil
}
