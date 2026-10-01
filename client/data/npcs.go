package clientdata

import (
	_ "embed"
	"fmt"
)

//go:embed npc_sprites.yml
var npcSpritesYAML []byte

// NPCSprite maps an NPC sprite ID to its sprite, sprite/npc/<Sprite>.
type NPCSprite struct {
	ID     uint32 `yaml:"id"`
	Sprite string `yaml:"sprite"`
}

// NPCSprites is npc_sprites.yml.
type NPCSprites struct {
	Sprites []NPCSprite `yaml:"sprites"`
}

// LoadNPCSprites parses and checks the embedded npc_sprites.yml.
func LoadNPCSprites() (*NPCSprites, error) { return ParseNPCSprites(npcSpritesYAML) }

// ParseNPCSprites parses and checks an NPC sprite table. Sprites are not
// references: a build without one draws nothing for that NPC.
func ParseNPCSprites(data []byte) (*NPCSprites, error) {
	var n NPCSprites
	if err := strictUnmarshal(data, &n); err != nil {
		return nil, fmt.Errorf("npc_sprites.yml: %w", err)
	}
	ids := map[uint32]bool{}
	for _, s := range n.Sprites {
		if ids[s.ID] {
			return nil, fmt.Errorf("npc_sprites.yml: sprite %d is listed twice", s.ID)
		}
		ids[s.ID] = true
		if err := checkName("npc_sprites.yml", "sprite/npc/"+s.Sprite, "sprite/npc/"); err != nil {
			return nil, err
		}
	}
	return &n, nil
}
