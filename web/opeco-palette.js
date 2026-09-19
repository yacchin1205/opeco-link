const palettes = [
  ["red", 0], ["orange", 30], ["yellow", 55], ["green", 120],
  ["cyan", 185], ["blue", 220], ["purple", 285], ["pink", 330],
];

export function opecoPalette(color) {
  if (typeof color !== "string" || !/^#[0-9a-fA-F]{6}$/.test(color)) return "blue";
  const channels = [1, 3, 5].map((offset) => parseInt(color.slice(offset, offset + 2), 16) / 255);
  const [red, green, blue] = channels;
  const maximum = Math.max(...channels);
  const delta = maximum - Math.min(...channels);
  if (maximum === 0 || delta / maximum < 0.08) return "blue";
  let hue;
  if (maximum === red) hue = ((green - blue) / delta) % 6;
  else if (maximum === green) hue = (blue - red) / delta + 2;
  else hue = (red - green) / delta + 4;
  hue = (hue * 60 + 360) % 360;
  const distance = (target) => Math.min(Math.abs(hue - target), 360 - Math.abs(hue - target));
  return palettes.reduce((nearest, candidate) => distance(candidate[1]) < distance(nearest[1]) ? candidate : nearest)[0];
}
