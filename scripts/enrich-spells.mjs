// One-time (re-runnable) enrichment: merges frontend seed-catalog export into
// internal/catalog/spells.json by spell id. Adds classes, concentration,
// damage, damageType, savingThrowAbility. Existing fields are preserved.
//
// Usage:
//   node scripts/enrich-spells.mjs --input <path-to-seed-catalog.export.json>
import { readFileSync, writeFileSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const idx = process.argv.indexOf('--input');
if (idx === -1 || !process.argv[idx + 1]) {
  console.error('error: --input <seed-catalog.export.json> is required');
  process.exit(1);
}
const inputPath = resolve(process.argv[idx + 1]);
const catalogPath = resolve(here, '../internal/catalog/spells.json');

const seeds = JSON.parse(readFileSync(inputPath, 'utf8'));
const spells = JSON.parse(readFileSync(catalogPath, 'utf8'));
const seedById = new Map(seeds.map((s) => [s.id, s]));

let enriched = 0;
let missed = 0;
for (const spell of spells) {
  const seed = seedById.get(spell.id);
  if (!seed) {
    missed++;
    console.warn(`no seed match for ${spell.id}`);
    continue;
  }
  if (Array.isArray(seed.classes) && seed.classes.length > 0) spell.classes = seed.classes;
  if (seed.concentration === true) spell.concentration = true;
  if (seed.damage) spell.damage = seed.damage;
  if (seed.damageType) spell.damageType = seed.damageType;
  if (seed.savingThrowAbility) spell.savingThrowAbility = seed.savingThrowAbility;
  enriched++;
}

writeFileSync(catalogPath, JSON.stringify(spells, null, 2) + '\n', 'utf8');
console.log(`enriched ${enriched} spells, ${missed} missed (of ${spells.length})`);
if (missed > 0) {
  console.error('FAIL: expected 0 missed (parity is established)');
  process.exit(1);
}
