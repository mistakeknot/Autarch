package specs

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
)

var idPattern = regexp.MustCompile(`^PRD-(\d+)\.ya?ml$`)

func NextID(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "PRD-001", nil
	}
	var nums []int
	for _, e := range entries {
		m := idPattern.FindStringSubmatch(e.Name())
		if len(m) == 2 {
			if n, err := strconv.Atoi(m[1]); err == nil {
				nums = append(nums, n)
			}
		}
	}
	sort.Ints(nums)
	next := 1
	if len(nums) > 0 {
		next = nums[len(nums)-1] + 1
	}
	return fmt.Sprintf("PRD-%03d", next), nil
}
