// Package heroes documents childhood-hero easter-egg aliases used across NexusERP.
//
// Each alias wraps a real domain function. The name is playful; the behavior is
// intentional and covered by the same tests as the underlying implementation.
//
// Roster (logic → hero):
//
//   AuthZ allow check     → He-Man      ByThePowerOfGrayskull  (power to act)
//   Permission + MFA      → Spider-Man  WithGreatPower         (responsibility)
//   Search tokenization   → Batman      DetectiveMode          (finds clues in text)
//   Accent folding        → Bat-Computer BatComputer           (normalizes evidence)
//   Image downscale       → Goku        Kamehameha             (concentrated transform)
//   Token hashing         → Sailor Moon MoonPrismPower         (identity → secret form)
//   Stock signed delta    → TMNT        CowabungaDelta         (shell-safe stock math)
//   Prefer CEDI/ARRIVAL   → Thundercats SightBeyondSight       (see the right warehouse)
//   Post inbound receipt  → Digimon     DigivolveReceipt       (DRAFT → POSTED evolution)
package heroes
