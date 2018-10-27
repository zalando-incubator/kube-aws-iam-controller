package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRoleStore(t *testing.T) {
	roleStore := NewRoleStore()
	require.False(t, roleStore.Exists("role", "namespace"))

	// Add and check
	roleStore.Add("role", "namespace", "pod_name")
	require.True(t, roleStore.Exists("role", "namespace"))

	roleStore.Add("role", "namespace2", "pod_name")
	require.True(t, roleStore.Exists("role", "namespace2"))

	roleStore.Add("role", "namespace", "pod_name2")
	require.True(t, roleStore.Exists("role", "namespace"))

	// Remove and check
	roleStore.Remove("role", "namespace", "pod_name")
	require.True(t, roleStore.Exists("role", "namespace"))

	roleStore.Remove("role", "namespace", "pod_name2")
	require.False(t, roleStore.Exists("role", "namespace"))
	require.True(t, roleStore.Exists("role", "namespace2"))

	roleStore.Remove("role", "namespace2", "pod_name")
	require.False(t, roleStore.Exists("role", "namespace2"))
}
