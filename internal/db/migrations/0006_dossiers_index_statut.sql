-- Index composite pour la liste filtree par statut, et pour son comptage.
--
-- Sans lui, filtrer par statut degenerait en un parcours de tous les dossiers
-- de l'agence : l'index (agence, cree_le) sert l'ordre mais laisse le statut
-- en filtre applique apres coup. Mesure sur un million de dossiers, dont
-- 200 000 pour l'agence interrogee : 259 ms pour rendre cinquante lignes, et
-- 72 ms pour les compter.
--
-- L'index partiel dossiers_instruction ne couvrait qu'un statut sur cinq ; il
-- devient redondant avec celui-ci et disparait.

CREATE INDEX dossiers_agence_statut
    ON dossiers (agence, statut, cree_le DESC);

DROP INDEX IF EXISTS dossiers_instruction;
