// Code generated by "stringer -type=Port -output=string.go"; DO NOT EDIT.

package ethtool

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[TwistedPair-0]
	_ = x[AUI-1]
	_ = x[MII-2]
	_ = x[Fibre-3]
	_ = x[BNC-4]
	_ = x[DirectAttach-5]
	_ = x[None-239]
	_ = x[Other-255]
}

const (
	_Port_name_0 = "TwistedPairAUIMIIFibreBNCDirectAttach"
	_Port_name_1 = "None"
	_Port_name_2 = "Other"
)

var (
	_Port_index_0 = [...]uint8{0, 11, 14, 17, 22, 25, 37}
)

func (i Port) String() string {
	switch {
	case 0 <= i && i <= 5:
		return _Port_name_0[_Port_index_0[i]:_Port_index_0[i+1]]
	case i == 239:
		return _Port_name_1
	case i == 255:
		return _Port_name_2
	default:
		return "Port(" + strconv.FormatInt(int64(i), 10) + ")"
	}
}
