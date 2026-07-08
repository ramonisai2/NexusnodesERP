/**
 * Childhood-hero easter eggs used as intentional aliases for real domain logic.
 * Stable public names; behavior matches the underlying helpers.
 */

export type HeroEgg = {
  hero: string;
  call: string;
  does: string;
  why: string;
};

/** Roster shown when Konami / hero phrase unlocks the panel. */
export const HERO_ROSTER: HeroEgg[] = [
  {
    hero: "He-Man",
    call: "ByThePowerOfGrayskull",
    does: "AuthZ allow (OPA / local policy)",
    why: "He draws the power to act — the subject asks if the action is allowed.",
  },
  {
    hero: "Spider-Man",
    call: "WithGreatPower",
    does: "Permission check",
    why: "With great power comes great responsibility — hold a permission before using it.",
  },
  {
    hero: "Batman",
    call: "DetectiveMode / BatComputer",
    does: "Search tokenize + accent fold",
    why: "The detective finds clues in messy text; the Bat-Computer normalizes evidence.",
  },
  {
    hero: "Goku",
    call: "Kamehameha",
    does: "Image downscale to web JPEG",
    why: "Concentrates raw photo energy into a controlled, lighter blast.",
  },
  {
    hero: "Sailor Moon",
    call: "MoonPrismPower",
    does: "Hash QR upload tokens",
    why: "Transformation hides the civilian identity — we store only the hashed form.",
  },
  {
    hero: "TMNT",
    call: "CowabungaDelta",
    does: "Signed stock quantity by movement type",
    why: "Keeps the warehouse in balance: inbound up, outbound down.",
  },
  {
    hero: "Thundercats",
    call: "SightBeyondSight",
    does: "Prefer ARRIVAL / CEDI warehouse",
    why: "Sword of Omens — see the right receiving bay beyond ordinary shelves.",
  },
  {
    hero: "Digimon",
    call: "DigivolveReceipt",
    does: "Post DRAFT inbound receipt",
    why: "A draft digivolves into posted RECEIPT movements and labels.",
  },
];

const KONAMI = [
  "ArrowUp",
  "ArrowUp",
  "ArrowDown",
  "ArrowDown",
  "ArrowLeft",
  "ArrowRight",
  "ArrowLeft",
  "ArrowRight",
  "b",
  "a",
];

/**
 * Digimon-style unlock: Konami code OR typing "digivolve" / "grayskull".
 * Returns a disposer for the keydown listener.
 */
export function listenForHeroUnlock(onUnlock: () => void): () => void {
  let idx = 0;
  let buffer = "";
  const onKey = (e: KeyboardEvent) => {
    const key = e.key.length === 1 ? e.key.toLowerCase() : e.key;
    if (key === KONAMI[idx] || (KONAMI[idx].length === 1 && key === KONAMI[idx])) {
      idx += 1;
      if (idx >= KONAMI.length) {
        idx = 0;
        onUnlock();
      }
    } else if (e.key === "ArrowUp") {
      idx = e.key === KONAMI[0] ? 1 : 0;
    } else {
      idx = 0;
    }

    if (e.key.length === 1 && /[a-z]/i.test(e.key)) {
      buffer = (buffer + e.key.toLowerCase()).slice(-12);
      if (buffer.includes("digivolve") || buffer.includes("grayskull") || buffer.includes("cowabunga")) {
        buffer = "";
        onUnlock();
      }
    }
  };
  window.addEventListener("keydown", onKey);
  return () => window.removeEventListener("keydown", onKey);
}
