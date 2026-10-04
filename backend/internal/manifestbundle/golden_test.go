package manifestbundle

// terranova-bundle-v2. Computed independently on PostgreSQL 17 with an
// equivalent SQL rendering of the v2 encoding (ORDER BY path COLLATE "C",
// mode 755 iff (mode & 73) <> 0); goldenFiles stored with mode 420, 509 and 0.
// The step 2 SQL backfill writes v1 (see LegacyHashV1 golden vectors).
const (
	goldenEmpty  = "34f65ca8d6000e40f17a78454f6444181e53f7e4f8c651f6ef005e25f95526c2"
	goldenBundle = "5f72d2a9e2427fd55f2f3ef572f39807b2b7bf986c69b90685c2cdd903e349f2"
)
