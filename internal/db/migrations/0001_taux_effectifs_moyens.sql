-- Taux effectifs moyens publies par arrete du ministre des finances, sur
-- proposition de la Banque Centrale de Tunisie, au dernier mois de chaque
-- semestre. Ils servent de reference pour le semestre suivant.
--
-- Le seuil du taux excessif n'est pas stocke : il se deduit du taux effectif
-- moyen par la regle du cinquieme, et cette regle est portee par le programme
-- COBOL. On ne conserve ici que la donnee publiee.

CREATE TABLE taux_effectifs_moyens (
    categorie   text        NOT NULL,
    -- Semestre d'application, sous la forme 2026S1.
    semestre    text        NOT NULL,
    -- En pourcent, deux decimales, comme les taux publies.
    tem         numeric(4,2) NOT NULL CHECK (tem > 0 AND tem < 100),
    -- Reference de l'arrete, pour la tracabilite reglementaire.
    arrete      text        NOT NULL,
    publie_le   date        NOT NULL,
    cree_le     timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (categorie, semestre)
);

COMMENT ON TABLE taux_effectifs_moyens IS
    'Taux effectifs moyens par categorie de concours et par semestre, loi n° 99-64 du 15 juillet 1999';

-- La recherche se fait toujours par categorie, semestre le plus recent.
CREATE INDEX taux_effectifs_moyens_recherche
    ON taux_effectifs_moyens (categorie, semestre DESC);
