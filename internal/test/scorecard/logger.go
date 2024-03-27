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
	v1 "k8s.io/api/core/v1"
)

type ContainerLog struct {
	Container string
	Log       string
}

func (r *TestResources) logContainer(namespace, podName, containerName string) {
	containerLog := &ContainerLog{
		Container: containerName,
	}
	buf := &strings.Builder{}

	err := r.GetContainerLogs(namespace, podName, containerName, buf)
	if err != nil {
		buf.WriteString(fmt.Sprintf("%s\n", err.Error()))
	}

	containerLog.Log = buf.String()
	r.LogChannel <- containerLog
}

func (r *TestResources) GetContainerLogs(namespace, podName, containerName string, dest io.Writer) error {
	ctx, cancel := context.WithTimeout(context.TODO(), testTimeout)
	defer cancel()

	logOptions := &v1.PodLogOptions{
		Follow:    true,
		Container: containerName,
	}
	stream, err := r.Client.CoreV1().Pods(namespace).GetLogs(podName, logOptions).Stream(ctx)
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

func (r *TestResources) CollectLogs() []*ContainerLog {
	logs := make([]*ContainerLog, 0)
	for i := 0; i < cap(r.LogChannel); i++ {
		logs = append(logs, <-r.LogChannel)
	}
	return logs
}

func (r *TestResources) CollectContainersLogsToResult() {
	logs := r.CollectLogs()
	for _, log := range logs {
		if log != nil {
			r.Log += fmt.Sprintf("\n%s CONTAINER LOG:\n\n\t%s\n", strings.ToUpper(log.Container), log.Log)
		}
	}
}

func (r *TestResources) StartLogs(cr *operatorv1beta1.Cryostat) error {
	podName, err := r.getCryostatPodNameForCR(cr)
	if err != nil {
		return fmt.Errorf("failed to get pod name for CR: %s", err.Error())
	}

	containerNames := []string{
		cr.Name,
		cr.Name + "-grafana",
		cr.Name + "-jfr-datasource",
	}

	r.LogChannel = make(chan *ContainerLog, len(containerNames))

	for _, containerName := range containerNames {
		go r.logContainer(cr.Namespace, podName, containerName)
	}

	return nil
}
