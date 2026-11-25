// Package kube is the small part of the Kubernetes API cxz uses: JSON objects
// by path, server-side apply, label-selected lists and exec. It is not a
// general client; it exists so the manager can run project pods without
// carrying client-go. See docs/plans/kubernetes-projects.md.
package kube

import (
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
)

const serviceAccountDir = "/var/run/secrets/kubernetes.io/serviceaccount"

// Config is where the API server is and how to prove who is asking.
type Config struct {
	Server    string
	CA        []byte
	Namespace string
	// TokenFile is re-read for every request: a projected service account
	// token is rotated under a running pod. Token is used when it is empty.
	TokenFile string
	Token     string
}

func (c Config) token() (string, error) {
	if c.TokenFile == "" {
		return c.Token, nil
	}
	b, err := os.ReadFile(c.TokenFile)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// InCluster is the pod's own service account.
func InCluster() (Config, error) {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return Config{}, fmt.Errorf("not running in a Kubernetes pod")
	}
	ca, err := os.ReadFile(filepath.Join(serviceAccountDir, "ca.crt"))
	if err != nil {
		return Config{}, err
	}
	ns, err := os.ReadFile(filepath.Join(serviceAccountDir, "namespace"))
	if err != nil {
		return Config{}, err
	}
	return Config{Server: "https://" + net.JoinHostPort(host, port), CA: ca, Namespace: strings.TrimSpace(string(ns)), TokenFile: filepath.Join(serviceAccountDir, "token")}, nil
}

// Kubeconfig reads the current context of a kubeconfig file. Only bearer
// token users are supported: that is what a service account gives, and a
// manager outside the cluster is for development, not deployment.
func Kubeconfig(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var f struct {
		Current  string `yaml:"current-context"`
		Clusters []struct {
			Name    string `yaml:"name"`
			Cluster struct {
				Server string `yaml:"server"`
				CAData string `yaml:"certificate-authority-data"`
				CAFile string `yaml:"certificate-authority"`
			} `yaml:"cluster"`
		} `yaml:"clusters"`
		Users []struct {
			Name string `yaml:"name"`
			User struct {
				Token     string `yaml:"token"`
				TokenFile string `yaml:"tokenFile"`
			} `yaml:"user"`
		} `yaml:"users"`
		Contexts []struct {
			Name    string `yaml:"name"`
			Context struct {
				Cluster   string `yaml:"cluster"`
				User      string `yaml:"user"`
				Namespace string `yaml:"namespace"`
			} `yaml:"context"`
		} `yaml:"contexts"`
	}
	if err = yaml.Unmarshal(b, &f); err != nil {
		return Config{}, fmt.Errorf("kubeconfig: %w", err)
	}
	var out Config
	found := false
	for _, ctx := range f.Contexts {
		if ctx.Name != f.Current {
			continue
		}
		found = true
		out.Namespace = ctx.Context.Namespace
		for _, c := range f.Clusters {
			if c.Name != ctx.Context.Cluster {
				continue
			}
			out.Server = c.Cluster.Server
			if c.Cluster.CAData != "" {
				if out.CA, err = base64.StdEncoding.DecodeString(c.Cluster.CAData); err != nil {
					return Config{}, fmt.Errorf("kubeconfig: certificate-authority-data: %w", err)
				}
			} else if c.Cluster.CAFile != "" {
				if out.CA, err = os.ReadFile(c.Cluster.CAFile); err != nil {
					return Config{}, err
				}
			}
		}
		for _, u := range f.Users {
			if u.Name == ctx.Context.User {
				out.Token, out.TokenFile = u.User.Token, u.User.TokenFile
			}
		}
	}
	if !found || out.Server == "" {
		return Config{}, fmt.Errorf("kubeconfig: current context %q has no cluster", f.Current)
	}
	if out.Token == "" && out.TokenFile == "" {
		return Config{}, fmt.Errorf("kubeconfig: current context's user has no bearer token")
	}
	if out.Namespace == "" {
		out.Namespace = "default"
	}
	return out, nil
}

// Load is the in-cluster account when there is one, otherwise $KUBECONFIG.
func Load() (Config, error) {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return InCluster()
	}
	path := os.Getenv("KUBECONFIG")
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".kube", "config")
	}
	return Kubeconfig(path)
}
