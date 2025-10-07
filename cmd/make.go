/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	mathrand "math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// makeCmd represents the make command
var makeCmd = &cobra.Command{
	Use:   "make",
	Short: "Start making secrets",
	Long:  `Makes secrets in the specified namespace as fast as possible.`,
	Run: func(cmd *cobra.Command, args []string) {
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				log.Fatalf("cannot determine user home dir: %v", err)
			}
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
		cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			log.Fatalf("cannot build kubeconfig: %v", err)
		}
		fmt.Printf("Using kubeconfig: %s\n", kubeconfig)
		ns, err := cmd.Flags().GetString("namespace")
		if err != nil {
			log.Fatalf("cannot get namespace flag: %v", err)
		}
		cRange, err := cmd.Flags().GetInt("namespace-count")
		if err != nil {
			log.Fatalf("cannot get namespace-count flag: %v", err)
		}
		fmt.Printf("Creating %d namespaces\n", cRange)
		client, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			log.Fatalf("cannot create k8s client: %v", err)
		}

		for i := range cRange {
			nn := fmt.Sprintf("%s-%02x", ns, i)
			client.DiscoveryClient = nil // Disable discovery to avoid extra round-trips to /api endpoints.
			// Disable discovery to avoid extra round-trips to /api endpoints.
			//client.DiscoveryClient = nil
			_, err = client.CoreV1().Namespaces().Create(context.Background(), &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nn,
				},
			}, metav1.CreateOptions{})
			if err != nil {
				log.Printf("ERR: cannot create namespace %s: %v", nn, err)
			} else {
				fmt.Printf(".")
			}
		}
		fmt.Println("")

		created := make(chan time.Duration)
		cWorkers, err := cmd.Flags().GetInt("workers")
		if err != nil {
			log.Fatalf("cannot get workers flag: %v", err)
		}
		for i := range cWorkers {
			go func() {
				// 2. Instantiate clientset.
				client, err := kubernetes.NewForConfig(cfg)
				if err != nil {
					log.Fatalf("cannot create k8s client: %v", err)
				}
				client.DiscoveryClient = nil // Disable discovery to avoid extra round-trips to /api endpoints.
				for {
					startTime := time.Now()
					nn := fmt.Sprintf("%s-%02x", ns, mathrand.Intn(cRange))
					if i%2 == 0 {
						makeConfigMap(client, nn)
					} else {
						makeSecret(client, nn)
					}
					created <- time.Since(startTime)
				}
			}()
		}
		go func() {
			tick := time.NewTicker(10 * time.Second)
			defer tick.Stop()
			c := 0
			n := 0
			for {
				select {
				case <-created:
					n++
					c++
				case <-tick.C:
					fmt.Printf("Created %d objects (%f objects/sec); %d objects so far\n", n, float64(n)/float64(10.0), c)
					n = 0
				}
			}
		}()
		select {}
	},
}

func init() {
	makeCmd.Flags().StringP("namespace", "n", "default", "Namespace prefix")
	makeCmd.Flags().IntP("namespace-count", "c", 256, "Number of namespaces to create and distribute secrets across")
	makeCmd.Flags().IntP("workers", "w", 8, "Number of worker goroutines")
	rootCmd.AddCommand(makeCmd)

}

func makeSecret(client *kubernetes.Clientset, ns string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("secret-%s", randomString()),
		},
		Type:       v1.SecretTypeOpaque,
		StringData: makeRandomData(256),
	}

	_, err := client.CoreV1().Secrets(ns).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		log.Printf("ERR: failed to create secret: %v", err)
	}
}

func makeConfigMap(client *kubernetes.Clientset, ns string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("cm-%s", randomString()),
		},
		Data: makeRandomData(256),
	}

	_, err := client.CoreV1().ConfigMaps(ns).Create(ctx, cm, metav1.CreateOptions{})
	if err != nil {
		log.Printf("ERR: failed to create configmap: %v", err)
	}
}

func randomString() string {
	b16 := make([]byte, 16)
	_, err := rand.Read(b16)
	if err != nil {
		log.Fatal("failed to read random bytes")
	}
	return fmt.Sprintf("%08x", b16)
}

func makeRandomData(size int) map[string]string {
	stringData := make(map[string]string, 0)

	for i := range size {
		key := fmt.Sprintf("key-%02x", i)
		valueBytes := make([]byte, 2048)
		_, err := rand.Read(valueBytes)
		if err != nil {
			log.Fatal("failed to read random bytes")
		}
		value := fmt.Sprintf("%x", valueBytes)
		stringData[key] = value
	}
	return stringData
}
