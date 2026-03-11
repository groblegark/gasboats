package podmanager

import corev1 "k8s.io/api/core/v1"

// buildDindSidecar returns a Docker-in-Docker sidecar using the K8s 1.29+
// native sidecar pattern (initContainer with restartPolicy: Always). The
// sidecar runs docker:dind in privileged mode with TLS disabled, exposing
// the Docker daemon on tcp://127.0.0.1:2375 for the agent container.
func (m *K8sManager) buildDindSidecar() corev1.Container {
	restartAlways := corev1.ContainerRestartPolicyAlways
	return corev1.Container{
		Name:  DindContainerName,
		Image: DindImage,
		Env: []corev1.EnvVar{
			{Name: "DOCKER_TLS_CERTDIR", Value: ""},
		},
		SecurityContext: &corev1.SecurityContext{
			Privileged:   boolPtr(true),
			RunAsUser:    intPtr(0),
			RunAsNonRoot: boolPtr(false),
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: VolumeDindStorage, MountPath: MountDindStorage},
		},
		RestartPolicy: &restartAlways,
	}
}
