// Melody choices shared by the chime list and the alert picker.

export const OFF = "off";
export const CUSTOM = "custom"; // a custom rtttl set in the config file

// melodyChoices lists the built-in melodies, then "off", then "custom"
// when the config currently uses a custom tune.
export function melodyChoices(melodies, offLabel, current) {
  const out = melodies.map(m => ({ name: m.name, label: m.label }));
  out.push({ name: OFF, label: offLabel });
  if (current === CUSTOM) out.push({ name: CUSTOM, label: "Custom tune from the config file" });
  return out;
}

export const playable = name => name !== OFF && name !== CUSTOM;
