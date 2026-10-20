package image

// PublicKey is the ed25519 key that signs image checksums. It is pinned in
// the binary so a compromised release host cannot swap the rootfs. Keep it
// in sync with scripts/image/pubkey.txt.
const PublicKey = "Divlxy4U2j7zl4k9Dp4IvUxk0g3Pq4dTEydN6Hfzh2s="
