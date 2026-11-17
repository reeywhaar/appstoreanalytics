import { describe, expect, it } from "vitest";

import { appColors } from "@app/charts/series";

describe("appColors", () => {
  it("gives the apps the palette in the order they were made", () => {
    const colors = appColors(["6445960440", "1557565886", "1578554636"]);
    expect(colors.get("1557565886")).toBe("var(--color-app-1)");
    expect(colors.get("1578554636")).toBe("var(--color-app-2)");
    expect(colors.get("6445960440")).toBe("var(--color-app-3)");
  });

  it("keeps an app's colour when a newer app comes", () => {
    const before = appColors(["1557565886", "1578554636"]);
    const after = appColors(["1557565886", "1578554636", "6758887371"]);
    expect(after.get("1557565886")).toBe(before.get("1557565886"));
    expect(after.get("1578554636")).toBe(before.get("1578554636"));
  });

  it("makes a colour from the id past the palette, the same each time", () => {
    const ids = Array.from({ length: 22 }, (_, i) => String(1000 + i));
    const colors = appColors(ids);
    expect(colors.get("1019")).toBe("var(--color-app-20)");
    expect(colors.get("1020")).toMatch(/^light-dark\(oklch/);
    expect(appColors(ids).get("1021")).toBe(colors.get("1021"));
    expect(colors.get("1020")).not.toBe(colors.get("1021"));
  });
});
