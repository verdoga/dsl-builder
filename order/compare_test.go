package order_test

import (
	"fmt"
	"math/big"
	"math/rand"
	"strings"
	"testing"

	"github.com/verdoga/dsl-builder/diagnostic"
	"github.com/verdoga/dsl-builder/order"
)

func TestCompareQA5ContractCases(t *testing.T) {
	cases := []struct {
		name        string
		left, right string
		want        int
	}{
		{"same_one", "1", "1", 0},
		{"one_and_01", "1", "01", 0},
		{"one_and_001", "1", "001", 0},
		{"01_and_001", "01", "001", 0},
		{"same_longer_value", "0001000", "01000", 0},
		{"less_single_digit", "1", "9", -1},
		{"greater_single_digit", "9", "1", 1},
		{"numeric_not_lexical", "9", "10", -1},
		{"zero_prefix_longer_but_smaller", "0000009", "10", -1},
		{"equal_raw_lengths_different_significant_lengths", "0010", "0009", 1},
		{"different_lengths_after_zero_prefix", "00099", "0100", -1},
		{"equal_significant_lengths_first_digit", "00019", "20", -1},
		{"equal_significant_lengths_middle_digit", "1029", "01039", -1},
		{"equal_significant_lengths_last_digit", "0012345678", "12345679", -1},
		{"not_character_difference", "100", "900", -1},
		{"signed_32_boundary", "2147483647", "2147483648", -1},
		{"unsigned_32_boundary", "4294967295", "4294967296", -1},
		{"signed_64_boundary", "9223372036854775807", "9223372036854775808", -1},
		{"unsigned_64_boundary", "18446744073709551615", "18446744073709551616", -1},
		{"beyond_unsigned_64_adjacent", "18446744073709551616", "18446744073709551617", -1},
		{"float64_exact_integer_boundary", "9007199254740992", "9007199254740993", -1},
		{"large_adjacent_decimal_values", "100000000000000000000000000000000000000", "100000000000000000000000000000000000001", -1},
		{"huge_same_value", strings.Repeat("0", 8192) + strings.Repeat("7", 8192), strings.Repeat("7", 8192), 0},
		{"huge_last_digit", "1" + strings.Repeat("0", 32766) + "1", "1" + strings.Repeat("0", 32766) + "2", -1},
		{"huge_carry", strings.Repeat("9", 32768), "1" + strings.Repeat("0", 32768), -1},
		{"huge_prefix_of_zeros", strings.Repeat("0", 32768) + "2", "10", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			compareQA5RequireValid(t, tc.left)
			compareQA5RequireValid(t, tc.right)
			compareQA5Assert(t, tc.left, tc.right, tc.want)
			compareQA5Assert(t, tc.right, tc.left, -tc.want)
			compareQA5Assert(t, tc.left, tc.left, 0)
			compareQA5Assert(t, tc.right, tc.right, 0)
		})
	}
}

func TestCompareQA5BigIntegerOracle(t *testing.T) {
	// math/big служит независимым числовым эталоном; данные воспроизводимы.
	weights := []string{"1", "01", "001", "9", "10", "010", "99", "100"}
	for _, bits := range []uint{31, 32, 53, 63, 64, 127, 128, 255, 1024} {
		power := new(big.Int).Lsh(big.NewInt(1), bits)
		for _, delta := range []int64{-1, 0, 1} {
			weight := new(big.Int).Add(power, big.NewInt(delta)).String()
			weights = append(weights, weight, "000"+weight)
		}
	}
	rng := rand.New(rand.NewSource(20261106))
	for _, length := range []int{1, 2, 3, 9, 16, 19, 20, 32, 64, 127, 256, 1024, 4096} {
		digits := make([]byte, length)
		digits[0] = byte('1' + rng.Intn(9))
		for i := 1; i < len(digits); i++ {
			digits[i] = byte('0' + rng.Intn(10))
		}
		weight := string(digits)
		weights = append(weights, weight, strings.Repeat("0", length+3)+weight)
	}

	numbers := make([]*big.Int, len(weights))
	for i, weight := range weights {
		compareQA5RequireValid(t, weight)
		var ok bool
		numbers[i], ok = new(big.Int).SetString(weight, 10)
		if !ok || numbers[i].Sign() <= 0 {
			t.Fatalf("недопустимые данные эталона: индекс %d", i)
		}
	}
	for i, left := range weights {
		t.Run(fmt.Sprintf("left_%03d_digits_%d", i, len(left)), func(t *testing.T) {
			for j, right := range weights {
				compareQA5Assert(t, left, right, numbers[i].Cmp(numbers[j]))
			}
		})
	}
}

func compareQA5RequireValid(t *testing.T, raw string) {
	t.Helper()
	if err := order.Validate(raw, diagnostic.Context{}); err != nil {
		t.Fatalf("корректная положительная ASCII-запись длиной %d отклонена: %v", len(raw), err)
	}
}

func compareQA5Assert(t *testing.T, left, right string, want int) {
	t.Helper()
	if got := order.Compare(left, right); got != want {
		t.Errorf("Compare(%s, %s) = %d; требуется ровно %d", compareQA5Describe(left), compareQA5Describe(right), got, want)
	}
}

func compareQA5Describe(raw string) string {
	if len(raw) <= 48 {
		return fmt.Sprintf("%q", raw)
	}
	return fmt.Sprintf("%q…%q (длина %d)", raw[:20], raw[len(raw)-20:], len(raw))
}
