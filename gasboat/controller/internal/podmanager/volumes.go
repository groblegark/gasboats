package podmanager

import corev1 "k8s.io/api/core/v1"

// buildVolumes returns the volumes for the pod based on role.
func (m *K8sManager) buildVolumes(spec AgentPodSpec) []corev1.Volume {
	var volumes []corev1.Volume

	// Workspace volume: PVC for persistent roles, EmptyDir for ephemeral.
	if spec.WorkspaceStorage != nil {
		claimName := spec.WorkspaceStorage.ClaimName
		if claimName == "" {
			claimName = spec.PodName() + "-workspace"
		}
		volumes = append(volumes, corev1.Volume{
			Name: VolumeWorkspace,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: claimName,
				},
			},
		})
	} else {
		volumes = append(volumes, corev1.Volume{
			Name: VolumeWorkspace,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	// Tmp volume: always EmptyDir.
	volumes = append(volumes, corev1.Volume{
		Name: VolumeTmp,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})

	// Beads config volume: ConfigMap mount if specified.
	if spec.ConfigMapName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: VolumeBeadsConfig,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: spec.ConfigMapName},
				},
			},
		})
	}

	// Docker-in-Docker storage volume.
	if spec.Docker {
		volumes = append(volumes, corev1.Volume{
			Name: VolumeDindStorage,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	// Claude credentials volume: Secret mount for OAuth token.
	if spec.CredentialsSecret != "" {
		volumes = append(volumes, corev1.Volume{
			Name: VolumeClaudeCreds,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: spec.CredentialsSecret,
				},
			},
		})
	}

	return volumes
}

// buildVolumeMounts returns the volume mounts for the agent container.
func (m *K8sManager) buildVolumeMounts(spec AgentPodSpec) []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{
		{Name: VolumeWorkspace, MountPath: MountWorkspace},
		{Name: VolumeTmp, MountPath: MountTmp},
	}

	if spec.ConfigMapName != "" {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      VolumeBeadsConfig,
			MountPath: MountBeadsConfig,
			ReadOnly:  true,
		})
	}

	// Claude state: mount workspace subPath at ~/.claude so Claude Code's
	// persistent memory (.claude/projects/*.jsonl) survives pod recreation.
	// Only for roles with persistent workspace storage (bd-48ary).
	if spec.WorkspaceStorage != nil {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      VolumeWorkspace,
			MountPath: MountClaudeState,
			SubPath:   SubPathClaudeState,
		})
	}

	// Claude credentials: mount secret to staging dir; entrypoint/gb copies to PVC.
	if spec.CredentialsSecret != "" {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      VolumeClaudeCreds,
			MountPath: MountClaudeCreds,
			ReadOnly:  true,
		})
	}

	return mounts
}
