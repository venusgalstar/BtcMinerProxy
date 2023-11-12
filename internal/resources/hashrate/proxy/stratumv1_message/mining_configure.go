package stratumv1_message

import (
	"encoding/json"

	"gitlab.com/TitanInd/proxy/proxy-router-v3/internal/resources/hashrate/proxy/interfaces"
)

// Message: {"method": "mining.configure","id": 1,"params": [["minimum-difficulty", "version-rolling"],{"minimum-difficulty.value": 2048, "version-rolling.mask": "1fffe000", "version-rolling.min-bit-count": 2}]}
const MethodMiningConfigure = "mining.configure"

type MiningConfigure struct {
	ID     int                    `json:"id"`
	Method string                 `json:"method,omitempty"`
	Params *miningConfigureParams `json:"params"`

	extParams *MiningConfigureExtensionParams
}

type miningConfigureParams = [2]json.RawMessage

type MiningConfigureExtensionParams struct {
	MinimumDifficulty         int    `json:"minimum-difficulty.value,omitempty"`
	VersionRollingMask        string `json:"version-rolling.mask,omitempty"`
	VersionRollingMinBitCount int    `json:"version-rolling.min-bit-count,omitempty"`
}

func NewMiningConfigure(ID int, extensions *MiningConfigureExtensionParams) *MiningConfigure {
	if extensions == nil {
		extensions = &MiningConfigureExtensionParams{}
	}
	return &MiningConfigure{
		ID:        ID,
		Method:    MethodMiningConfigure,
		extParams: extensions,
	}
}

func ParseMiningConfigure(b []byte) (*MiningConfigure, error) {
	m := &MiningConfigure{
		extParams: &MiningConfigureExtensionParams{},
	}
	err := json.Unmarshal(b, m)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(m.Params[1], m.extParams)
	if err != nil {
		return nil, err
	}

	return m, nil
}

func (m *MiningConfigure) GetID() int {
	return m.ID
}

func (m *MiningConfigure) SetID(ID int) {
	m.ID = ID
}

func (m *MiningConfigure) GetVersionRolling() (string, int) {
	return m.extParams.VersionRollingMask, m.extParams.VersionRollingMinBitCount
}

func (m *MiningConfigure) SetVersionRolling(mask string, minBitCount int) {
	m.extParams.VersionRollingMask = mask
	m.extParams.VersionRollingMinBitCount = minBitCount
}

func (m *MiningConfigure) GetMinimumDifficulty() int {
	return m.extParams.MinimumDifficulty
}

func (m *MiningConfigure) SetMinimumDifficulty(minimumDifficulty int) {
	m.extParams.MinimumDifficulty = minimumDifficulty
}

func (m *MiningConfigure) Serialize() []byte {
	extensions := []string{}
	if m.extParams.VersionRollingMask != "" {
		extensions = append(extensions, "version-rolling")
	}
	if m.extParams.MinimumDifficulty != 0 {
		extensions = append(extensions, "minimum-difficulty")
	}

	ext, _ := json.Marshal(extensions)
	param, _ := json.Marshal(m.extParams)

	m.Params = &[2]json.RawMessage{ext, param}
	res, _ := json.Marshal(m)

	return res
}

var _ interfaces.MiningMessageWithID = new(MiningConfigure)
