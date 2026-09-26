package sessionalias

import "strings"

const (
	MinLen = 3
	// A session runtime ID is 24 hexadecimal characters, and every one of them
	// is also legal in an alias. Staying well short of that length keeps the two
	// tellable apart by length alone, which is what resourceclient.sr relies on
	// to decide whether an identifier names an alias or an ID.
	MaxLen = 20
)

// Rule states Valid in the words a user gets back when they break it.
const Rule = "alias must be 3–20 characters: lowercase letters, digits and single hyphens, beginning with a letter"

// Valid mirrors payday's alias grammar, which the resource layer enforces
// underneath this one, less that grammar's 63-character limit. Underscore is
// absent there so that an alias can still be a DNS label, and widening it here
// would only move the rejection to a less helpful place.
func Valid(s string) bool {
	if len(s) < MinLen || len(s) > MaxLen {
		return false
	}
	if s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for i := 1; i < len(s); i++ {
		switch c := s[i]; {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '-':
			// Hyphens join nonempty groups: never doubled, never last.
			if i == len(s)-1 || s[i+1] == '-' {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// Common English words: no numeric suffixes or invented syllables.
var Words = strings.Fields(`
acorn amber anchor apple apricot apron arbor arch arrow ash atlas autumn
badge bamboo banana barrel basil basket bay beacon beach bean bear beaver
beech berry birch bison blade bloom blossom blue bluff boat book border
bow bowl branch brass bread breeze brick bridge brook brush bubble bucket
bud buffalo butter button cabin cactus cake camera candle candy canyon
cape carbon carrot castle cedar celery chalk cherry chess chest chime
chip circle citrus clay cliff clock cloud clover coast cobalt cocoa comet
cone cookie copper coral cotton crane creek crest cricket crown crystal
cube cup curtain cypress daisy dance dawn deer delta desert diamond dove
dragon dream drift drum duck dune dusk eagle earth echo elm ember emerald
falcon family feather fern field fig finch fir fire flame flannel flask
fleet flint flora flower flute foam forest fork fossil fox frame
frost fruit galaxy garden garnet gate gem ginger glass globe glow gold
goose grape grass gravel green grove gull harbor hare harp harvest haven
hazel heart heath hedge heron hill hive honey horse ice icicle indigo
iris island ivory jade jasmine jay jewel jigsaw juniper kettle key kiwi
koala lagoon lake lamp lantern larch lark laurel lava leaf leather lemon
leopard letter lilac lily lime linen lion lizard llama lotus maple marble
marina marsh meadow melon metal meteor mica mint mirror mist moon moss
mouse mud music nectar needle nest nettle nickel night
nut oak oasis ocean olive onyx opal orange orchid otter owl oyster paddle
paint palace palm panda paper parrot pastel path peach pearl pebble pecan
pelican pepper petal piano pigeon pine pink planet plum plume pocket pond
poppy porch prairie prism pulse pumice purple quartz quill quilt rabbit
rain rainbow raven ray reed reef ribbon rice ridge ripple river robin rock
rocket rose rowan ruby rye saddle sage sail salmon salt sand satin scale
scarlet scarf sea seal seed shadow shale shark sheep shell shield shore
silk silver slate sleet slope smoke snail snake snow soap solar song
sparrow spice spider spike sponge spoon spruce square star steam
steel stem stone stork storm straw stream string sugar summer summit sun
sunset surf swan sweet swift table tea teal thistle thorn thread thyme
tide tiger timber tin toast topaz torch tower trail tree tulip turtle
valley velvet violet vista voice walnut wave wax wheat wheel willow wind
window wing winter wolf wood wool wren yacht yam yarn yellow zebra zinc
`)
