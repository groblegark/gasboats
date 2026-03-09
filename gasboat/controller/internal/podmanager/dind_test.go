package podmanager

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestBuildPod_DockerDisabled(t *testing.T) {
	m := newTestManager()
	spec := minimalSpec()
	spec.Docker = false
	pod := m.buildPod(spec)

	for _, ic := range pod.Spec.InitContainers {
		if ic.Name == DindContainerName {
			t.Fatal("DinD sidecar should not be present when Docker is disabled")
		}
	}
	for _, v := range pod.Spec.Volumes {
		if v.Name == VolumeDindStorage {
			t.Fatal("dind-storage volume should not be present when Docker is disabled")
		}
	}
	for _, env := range pod.Spec.Containers[0].Env {
		if env.Name == "DOCKER_HOST" {
			t.Fatal("DOCKER_HOST should not be set when Docker is disabled")
		}
	}
}

func TestBuildPod_DockerEnabled(t *testing.T) {
	m := newTestManager()
	spec := minimalSpec()
	spec.Docker = true
	pod := m.buildPod(spec)

	// Verify DinD sidecar is present as a native sidecar (initContainer).
	var dind *corev1.Container
	for i := range pod.Spec.InitContainers {
		if pod.Spec.InitContainers[i].Name == DindContainerName {
			dind = &pod.Spec.InitContainers[i]
			break
		}
	}
	if dind == nil {
		t.Fatal("DinD sidecar not found in initContainers")
	}

	// Verify image.
	if dind.Image != DindImage {
		t.Errorf("DinD image = %q, want %q", dind.Image, DindImage)
	}

	// Verify restartPolicy: Always (K8s 1.29+ native sidecar pattern).
	if dind.RestartPolicy == nil || *dind.RestartPolicy != corev1.ContainerRestartPolicyAlways {
		t.Error("DinD sidecar should have restartPolicy: Always")
	}

	// Verify privileged mode.
	if dind.SecurityContext == nil || dind.SecurityContext.Privileged == nil || !*dind.SecurityContext.Privileged {
		t.Error("DinD sidecar should run in privileged mode")
	}

	// Verify DOCKER_TLS_CERTDIR="" env var.
	foundTLSEnv := false
	for _, env := range dind.Env {
		if env.Name == "DOCKER_TLS_CERTDIR" && env.Value == "" {
			foundTLSEnv = true
		}
	}
	if !foundTLSEnv {
		t.Error("DinD sidecar should have DOCKER_TLS_CERTDIR=\"\" env var")
	}

	// Verify dind-storage volume mount.
	foundMount := false
	for _, vm := range dind.VolumeMounts {
		if vm.Name == VolumeDindStorage && vm.MountPath == MountDindStorage {
			foundMount = true
		}
	}
	if !foundMount {
		t.Errorf("DinD sidecar should mount %s at %s", VolumeDindStorage, MountDindStorage)
	}

	// Verify dind-storage volume exists on the pod.
	foundVolume := false
	for _, v := range pod.Spec.Volumes {
		if v.Name == VolumeDindStorage && v.VolumeSource.EmptyDir != nil {
			foundVolume = true
		}
	}
	if !foundVolume {
		t.Error("Pod should have dind-storage emptyDir volume")
	}

	// Verify DOCKER_HOST is set on the agent container.
	foundDockerHost := false
	for _, env := range pod.Spec.Containers[0].Env {
		if env.Name == "DOCKER_HOST" && env.Value == "tcp://127.0.0.1:2375" {
			foundDockerHost = true
		}
	}
	if !foundDockerHost {
		t.Error("Agent container should have DOCKER_HOST=tcp://127.0.0.1:2375")
	}
}
