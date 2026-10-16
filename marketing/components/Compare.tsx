type Kind = "yes" | "no" | "partial" | "soon";
type Cell = { kind: Kind; label: string };

const columns = ["Un tableur", "L’appli de ma banque", "Une appli d’agrégation", "Lagune"] as const;

const yes = (label = "Oui"): Cell => ({ kind: "yes", label });
const no = (label = "Non"): Cell => ({ kind: "no", label });
const partial = (label: string): Cell => ({ kind: "partial", label });
const soon = (label: string): Cell => ({ kind: "soon", label });

/** Comparaison par *type d'approche*, jamais par marque : chaque case doit rester
 * vraie en général. « Variable » signifie « dépend des offres » — on ne prétend
 * rien sur un concurrent précis (publicité comparative : vérifiable et loyale). */
const rows: { label: string; cells: [Cell, Cell, Cell, Cell] }[] = [
  { label: "Catégories automatiques qui s’améliorent", cells: [no("À la main"), partial("Limitées"), yes(), yes("Oui, elles apprennent")] },
  { label: "Prévisions de solde", cells: [partial("À construire"), partial("Rares"), partial("Variable"), yes()] },
  { label: "Opérations récurrentes détectées", cells: [no(), partial("Variable"), partial("Variable"), yes()] },
  { label: "Vos identifiants bancaires restent chez vous", cells: [yes(), yes(), partial("Selon les cas"), yes("Oui, aujourd’hui")] },
  { label: "Aucune publicité, aucune revente de données", cells: [yes(), partial("Variable"), partial("Variable"), yes()] },
  { label: "Export complet et suppression en un geste", cells: [yes("Vos fichiers"), partial("Rare"), partial("Variable"), yes()] },
  { label: "Partage avec un proche, révocable", cells: [partial("À la main"), partial("Rare"), partial("Variable"), yes()] },
  { label: "Synchronisation automatique de la banque", cells: [no(), yes(), yes(), soon("En développement")] },
];

const symbol: Record<Kind, string> = { yes: "✓", no: "✕", partial: "≈", soon: "◔" };

export function Compare() {
  return (
    <div className="compare-wrap" tabIndex={0} role="region" aria-label="Tableau comparatif, défilable horizontalement">
      <table className="compare" role="table">
        <caption className="sr-only">
          Lagune comparée à un tableur, à l’appli de sa banque et à une appli d’agrégation
        </caption>
        <thead role="rowgroup">
          <tr role="row">
            <td role="presentation" />
            {columns.map((c, i) => (
              <th scope="col" role="columnheader" key={c} className={i === columns.length - 1 ? "us" : undefined}>
                {c}
              </th>
            ))}
          </tr>
        </thead>
        <tbody role="rowgroup">
          {rows.map((row) => (
            <tr key={row.label} role="row">
              <th scope="row" role="rowheader">{row.label}</th>
              {row.cells.map((cell, i) => (
                <td key={i} role="cell" data-label={columns[i]} className={i === row.cells.length - 1 ? "us" : undefined}>
                  <span className={`mark ${cell.kind}`}>
                    <span aria-hidden="true">{symbol[cell.kind]}</span>
                    {cell.label}
                  </span>
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
