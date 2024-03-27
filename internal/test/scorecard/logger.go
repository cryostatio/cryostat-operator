// Copyright The Cryostat Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package scorecard

import (
	"context"
	"fmt"
	"io"
	"strings"

	operatorv1beta1 "github.com/cryostatio/cryostat-operator/api/v1beta1"
	scapiv1alpha3 "github.com/operator-framework/api/pkg/apis/scorecard/v1alpha3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

type ContainerLog struct {
	Container string
	Log       string
}

func LogContainer(clientset *kubernetes.Clientset, namespace, podName, containerName string, ch chan *ContainerLog) {
	containerLog := &ContainerLog{
		Container: containerName,
	}
	buf := &strings.Builder{}

	err := GetContainerLogs(clientset, namespace, podName, containerName, buf)
	if err != nil {
		buf.WriteString(fmt.Sprintf("%s\n", err.Error()))
	}

	containerLog.Log = buf.String()
	ch <- containerLog
}

func GetContainerLogs(clientset *kubernetes.Clientset, namespace, podName, containerName string, dest io.Writer) error {
	ctx, cancel := context.WithTimeout(context.TODO(), testTimeout)
	defer cancel()

	logOptions := &v1.PodLogOptions{
		Follow:    true,
		Container: containerName,
	}
	stream, err := clientset.CoreV1().Pods(namespace).GetLogs(podName, logOptions).Stream(ctx)
	if err != nil {
		return fmt.Errorf("failed to get logs for container %s in pod %s: %s", containerName, podName, err.Error())
	}
	defer stream.Close()

	_, err = io.Copy(dest, stream)
	if err != nil {
		return fmt.Errorf("failed to store logs for container %s in pod %s: %s", containerName, podName, err.Error())
	}
	return nil
}

func CollectLogs(ch chan *ContainerLog) []*ContainerLog {
	logs := make([]*ContainerLog, 0)
	for i := 0; i < cap(ch); i++ {
		logs = append(logs, <-ch)
	}
	return logs
}

func CollectContainersLogsToResult(result *scapiv1alpha3.TestResult, ch chan *ContainerLog) {
	logs := CollectLogs(ch)
	for _, log := range logs {
		if log != nil {
			result.Log += fmt.Sprintf("\n%s CONTAINER LOG:\n\n\t%s\n", strings.ToUpper(log.Container), log.Log)
		}
	}
}

func StartLogs(clientset *kubernetes.Clientset, cr *operatorv1beta1.Cryostat) (chan *ContainerLog, error) {
	podName, err := getCryostatPodNameForCR(clientset, cr)
	if err != nil {
		return nil, fmt.Errorf("failed to get pod name for CR: %s", err.Error())
	}

	containerNames := []string{
		cr.Name,
		cr.Name + "-grafana",
		cr.Name + "-jfr-datasource",
	}

	ch := make(chan *ContainerLog, len(containerNames))

	for _, containerName := range containerNames {
		go LogContainer(clientset, cr.Namespace, podName, containerName, ch)
	}

	return ch, nil
}
