package car

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRemoveString(t *testing.T) {
	t.Run("removes string from slice when found", func(t *testing.T) {
		slice := []string{"a", "b", "c", "d"}
		result := removeString(slice, "b")
		assert.Equal(t, []string{"a", "c", "d"}, result)
	})

	t.Run("removes first occurrence only", func(t *testing.T) {
		slice := []string{"a", "b", "a", "c"}
		result := removeString(slice, "a")
		assert.Equal(t, []string{"b", "a", "c"}, result)
	})

	t.Run("returns original slice when string not found", func(t *testing.T) {
		slice := []string{"a", "b", "c"}
		result := removeString(slice, "d")
		assert.Equal(t, []string{"a", "b", "c"}, result)
		assert.Equal(t, slice, result, "Should return same slice reference when not found")
	})

	t.Run("handles empty slice", func(t *testing.T) {
		slice := []string{}
		result := removeString(slice, "a")
		assert.Equal(t, []string{}, result)
	})

	t.Run("removes from single element slice", func(t *testing.T) {
		slice := []string{"a"}
		result := removeString(slice, "a")
		assert.Equal(t, []string{}, result)
	})

	t.Run("removes from single element slice when not found", func(t *testing.T) {
		slice := []string{"a"}
		result := removeString(slice, "b")
		assert.Equal(t, []string{"a"}, result)
	})
}
