-- Piste d'audit des simulations. Un service qui produit des offres de credit
-- doit pouvoir dire qui a demande quoi, quand, et sur quel bareme le verdict
-- a ete rendu.
--
-- La cle d'API n'est pas conservee : seule une empreinte tronquee l'est, qui
-- permet de distinguer les appelants sans rien reveler du secret.

CREATE TABLE simulations (
    id            bigserial    PRIMARY KEY,
    -- Identifiant de correlation, celui du X-Request-Id de la reponse.
    requete_id    text         NOT NULL,
    empreinte_cle text,
    adresse       inet,

    -- La demande telle que le service l'a comprise, apres normalisation.
    demande       jsonb        NOT NULL,

    -- Le resultat, reduit a ce qui engage : les montants detailles se
    -- recalculent a l'identique a partir de la demande.
    teg           numeric(4,2) NOT NULL,
    cout_credit   numeric(18,3) NOT NULL,
    premiere_mensualite numeric(16,3) NOT NULL,

    -- Verdict de taux excessif, nul quand aucun n'a ete demande.
    tem           numeric(4,2),
    seuil         numeric(4,2),
    conforme      boolean,
    -- Bareme applique, pour que le verdict reste rattachable a son arrete.
    categorie     text,
    semestre      text,
    arrete        text,

    cree_le       timestamptz  NOT NULL DEFAULT now()
);

COMMENT ON TABLE simulations IS
    'Piste d''audit : une ligne par echeancier produit';

-- La consultation se fait du plus recent au plus ancien.
CREATE INDEX simulations_chronologie ON simulations (cree_le DESC);
-- Retrouver une simulation depuis l'identifiant rendu au client.
CREATE INDEX simulations_requete ON simulations (requete_id);
-- Retrouver les prets juges excessifs, ce que demande un controle.
CREATE INDEX simulations_non_conformes ON simulations (cree_le DESC)
    WHERE conforme IS FALSE;
