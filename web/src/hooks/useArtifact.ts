import { useEffect, useState } from "react";
import { api } from "../lib/api";
import type { ArtifactContentView } from "../types";

export function useArtifact(artifactID: string) {
  const [artifactView, setArtifactView] = useState<ArtifactContentView | null>(null);
  const [artifactError, setArtifactError] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!artifactID) {
      setArtifactView(null);
      return;
    }
    setLoading(true);
    setArtifactError("");
    void api
      .getArtifactContent(artifactID)
      .then(setArtifactView)
      .catch((error: Error) => setArtifactError(error.message))
      .finally(() => setLoading(false));
  }, [artifactID]);

  return { artifactView, artifactError, loading };
}
