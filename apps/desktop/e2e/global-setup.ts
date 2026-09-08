import { sweepLeakedHomes } from './homes';

// Runs once before the suite; see sweepLeakedHomes in homes.ts.
export default function globalSetup(): void {
  sweepLeakedHomes();
}
