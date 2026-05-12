import sharp from "sharp";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { mkdirSync } from "node:fs";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const src = path.join(__dirname, "../public/square_logo_200.png");
const dest = path.join(__dirname, "../public/icons");

mkdirSync(dest, { recursive: true });

const sizes = [
  { name: "icon-180.png", size: 180 },
  { name: "icon-192.png", size: 192 },
  { name: "icon-512.png", size: 512 },
];

for (const { name, size } of sizes) {
  await sharp(src)
    .resize(size, size, { fit: "contain", background: { r: 26, g: 26, b: 46, alpha: 1 } })
    .png()
    .toFile(path.join(dest, name));
  console.log(`Generated ${name} (${size}x${size})`);
}
