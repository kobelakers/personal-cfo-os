import type { ReactNode } from "react";

export function MetaChip(props: { label: string; value: string }) {
  return (
    <div className="meta-chip">
      <span>{props.label}</span>
      <strong>{props.value}</strong>
    </div>
  );
}

export function Panel(props: { title: string; subtitle?: string; children: ReactNode }) {
  return (
    <article className="panel panel-large">
      <header className="panel-header">
        <div>
          <h2>{props.title}</h2>
          {props.subtitle ? <p>{props.subtitle}</p> : null}
        </div>
      </header>
      {props.children}
    </article>
  );
}

export function KeyValueGrid(props: { items: Array<[string, string]> }) {
  return (
    <dl className="kv-grid">
      {props.items.map(([key, value]) => (
        <div key={key} className="kv-item">
          <dt>{key}</dt>
          <dd>{value}</dd>
        </div>
      ))}
    </dl>
  );
}

export function SectionList(props: { title: string; items: string[] }) {
  if (!props.items || props.items.length === 0) {
    return null;
  }
  return (
    <section className="section-list">
      <h3>{props.title}</h3>
      <ul>
        {props.items.map((item) => (
          <li key={item}>{item}</li>
        ))}
      </ul>
    </section>
  );
}

export function EmptyState(props: { children: ReactNode }) {
  return <p className="empty-state">{props.children}</p>;
}
