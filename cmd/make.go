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
		fmt.Printf("Creating secrets in namespace: %s\n", ns)

		created := make(chan time.Duration)
		for range 8 {
			go func() {
				// 2. Instantiate clientset.
				client, err := kubernetes.NewForConfig(cfg)
				if err != nil {
					log.Fatalf("cannot create k8s client: %v", err)
				}
				client.DiscoveryClient = nil // Disable discovery to avoid extra round-trips to /api endpoints.
				for {
					startTime := time.Now()
					makeSecrets(client, ns)
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
					fmt.Printf("Created %d secrets (%f secrets/sec); %d secrets so far\n", n, float64(n)/float64(10.0), c)
					n = 0
				}
			}
		}()
		select {}
	},
}

func init() {
	makeCmd.Flags().StringP("namespace", "n", "default", "Kubernetes namespace to create secrets in")
	rootCmd.AddCommand(makeCmd)

}

func makeSecrets(client *kubernetes.Clientset, ns string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 4. Generate random secret name.
	b16 := make([]byte, 16)
	_, err := rand.Read(b16)
	if err != nil {
		panic("failed to read random bytes")
	}
	secretName := fmt.Sprintf("secret-%08x", b16)
	stringData := make(map[string]string, 0)

	numberOfKeys := 1 + mathrand.Intn(90) // between 10 and 100 keys

	for i := range numberOfKeys {
		key := fmt.Sprintf("key-%02d", i)
		valueBytes := make([]byte, 256)
		_, err := rand.Read(valueBytes)
		if err != nil {
			log.Fatal("failed to read random bytes")
		}
		value := fmt.Sprintf("%x", valueBytes)
		stringData[key] = value
	}

	// 5. Build Secret object.
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: secretName,
		},
		Type:       v1.SecretTypeOpaque,
		StringData: stringData,
	}

	// 6. Create Secret.
	_, err = client.CoreV1().Secrets(ns).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		log.Printf("ERR: failed to create secret: %v", err)
	}
}
