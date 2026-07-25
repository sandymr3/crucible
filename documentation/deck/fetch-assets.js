// Downloads the tech-stack brand marks and rasterises them to PNG.
//
// Simple Icons ships CC0 SVG files; the marks themselves remain their owners'.
// Nominative use in a tech-stack slide is standard practice.
//
// Everything is recoloured to a single tint at fetch time (the CDN takes the
// colour in the path) so the logo strip reads as one designed row rather than
// eight competing brand palettes on the white background.

const fs = require("fs");
const path = require("path");
const sharp = require("sharp");

const TINT = "3A3D4D"; // slate — the logo strip sits on the white slide
const OUT = path.join(__dirname, "assets");
const SIZE = 320; // well above the 256px floor; these render small but crisp

const LOGOS = [
  { slug: "go", label: "Go" },
  { slug: "googlecloud", label: "Cloud Run" },
  { slug: "googlegemini", label: "Vertex AI" },
  { slug: "firebase", label: "Firebase" },
  { slug: "docker", label: "Docker" },
  { slug: "react", label: "React" },
  { slug: "typescript", label: "TypeScript" },
  { slug: "vite", label: "Vite" },
];

async function main() {
  fs.mkdirSync(OUT, { recursive: true });

  for (const { slug, label } of LOGOS) {
    const url = `https://cdn.simpleicons.org/${slug}/${TINT}`;
    const res = await fetch(url);
    if (!res.ok) throw new Error(`${slug}: HTTP ${res.status}`);
    const svg = Buffer.from(await res.arrayBuffer());

    // `density` drives the SVG rasterisation resolution; without it sharp
    // renders at the SVG's nominal size and upscaling produces soft edges.
    const png = await sharp(svg, { density: 400 })
      .resize(SIZE, SIZE, { fit: "contain", background: { r: 0, g: 0, b: 0, alpha: 0 } })
      .png()
      .toBuffer();

    fs.writeFileSync(path.join(OUT, `${slug}.png`), png);
    console.log(`  ${label.padEnd(12)} ${slug}.png  ${png.length} bytes`);
  }

  console.log(`\n${LOGOS.length} logos written to ${OUT}`);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
