import { describe, expect, it } from "vitest";
import { opecoPalette } from "../web/opeco-palette.js";

describe("opeco session palette", () => {
  it.each([
    ["#ff0000", "red"], ["#ff8000", "orange"], ["#ffea00", "yellow"],
    ["#00ff00", "green"], ["#00eaff", "cyan"], ["#0055ff", "blue"],
    ["#bf00ff", "purple"], ["#ff0080", "pink"],
    ["#f2d0d0", "red"], ["#f2e1d0", "orange"], ["#d0f2d0", "green"], ["#d0eff2", "cyan"],
    ["#ff0010", "red"], ["#00EAFF", "cyan"],
  ])("matches the iOS hue selection for %s", (color, palette) => {
    expect(opecoPalette(color)).toBe(palette);
  });

  it.each([null, undefined, "", "invalid", "#fff", "#888888", "#000000", "#ffffff", "#f2f0ef"])(
    "uses the iOS default blue for %s", (color) => {
      expect(opecoPalette(color)).toBe("blue");
    },
  );
});
