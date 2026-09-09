-- Dossiers de pret.
--
-- Aucune donnee a caractere personnel n'y figure : ni nom, ni CIN, ni revenu.
-- Le dossier porte la reference que la banque emploie deja dans son propre
-- systeme, ou l'identite reste. Ce choix garde l'application hors du champ
-- des obligations de la loi n° 2004-63 relative a la protection des donnees a
-- caractere personnel : declaration a l'INPDP, consentement, droit d'acces et
-- de rectification, duree de conservation.
--
-- Si l'identite devait un jour entrer ici, ces obligations s'appliqueraient et
-- le schema devrait porter le consentement horodate et une purge.

CREATE TABLE dossiers (
    id          bigserial   PRIMARY KEY,
    -- Reference du dossier dans le systeme de la banque. C'est la seule
    -- passerelle vers l'identite de l'emprunteur, et elle reste chez elle.
    reference   text        NOT NULL,

    -- Auteur du dossier, et agence de rattachement. L'agence est recopiee
    -- plutot que lue par jointure : muter un agent ne doit pas deplacer les
    -- dossiers qu'il a montes.
    agent_id    bigint      NOT NULL REFERENCES agents (id),
    agence      text        NOT NULL,

    statut      text        NOT NULL DEFAULT 'brouillon'
                CHECK (statut IN ('brouillon', 'en_instruction', 'accorde',
                                  'refuse', 'annule')),

    -- Les parametres du pret, tels qu'ils seront passes au moteur. Ils sont
    -- stockes en texte a l'echelle du millime plutot qu'en nombre flottant :
    -- c'est la meme discipline que dans le reste du service.
    capital        numeric(14,3) NOT NULL CHECK (capital > 0),
    taux           numeric(8,6)  NOT NULL CHECK (taux >= 0),
    mois           integer       NOT NULL CHECK (mois > 0),
    methode        text          NOT NULL DEFAULT 'annuite_constante',
    differe        integer       NOT NULL DEFAULT 0 CHECK (differe >= 0),
    type_differe   text          NOT NULL DEFAULT 'partiel'
                   CHECK (type_differe IN ('partiel', 'total')),
    categorie      text,

    -- Derniere simulation rattachee, qui porte le verdict de taux excessif.
    simulation_id bigint REFERENCES simulations (id) ON DELETE SET NULL,

    cree_le     timestamptz NOT NULL DEFAULT now(),
    maj_le      timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE dossiers IS
    'Dossiers de pret ; aucune donnee a caractere personnel';

-- La reference est unique au sein d'une agence : deux agences peuvent employer
-- la meme numerotation sans se gener.
CREATE UNIQUE INDEX dossiers_reference ON dossiers (agence, lower(reference));
-- La liste se consulte du plus recent au plus ancien, par agence.
CREATE INDEX dossiers_agence ON dossiers (agence, cree_le DESC);
CREATE INDEX dossiers_agent ON dossiers (agent_id, cree_le DESC);
-- Retrouver les dossiers en attente de decision.
CREATE INDEX dossiers_instruction ON dossiers (agence, cree_le DESC)
    WHERE statut = 'en_instruction';

-- Historique des changements de statut. Un dossier de credit doit pouvoir dire
-- qui a decide quoi, et quand.
CREATE TABLE dossiers_evenements (
    id          bigserial   PRIMARY KEY,
    dossier_id  bigint      NOT NULL REFERENCES dossiers (id) ON DELETE CASCADE,
    -- L'auteur de la decision reste, meme si son compte est desactive.
    agent_id    bigint      NOT NULL REFERENCES agents (id),

    statut_avant text,
    statut_apres text       NOT NULL,
    note         text,

    cree_le     timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE dossiers_evenements IS
    'Qui a fait passer un dossier d''un statut a un autre, et quand';

CREATE INDEX dossiers_evenements_dossier
    ON dossiers_evenements (dossier_id, cree_le);
