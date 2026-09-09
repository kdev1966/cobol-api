-- Comptes des agents de credit, et leurs sessions.
--
-- Deux secrets ne sont jamais conserves en clair : le mot de passe, dont seule
-- l'empreinte argon2id est gardee, et le jeton de session, dont seule
-- l'empreinte SHA-256 l'est. Une base derobee ne permet donc ni de se
-- connecter, ni de reprendre une session en cours.
--
-- Il n'y a pas d'auto-inscription : les comptes sont crees en ligne de
-- commande par un administrateur. Ouvrir une route de creation publique sur un
-- service qui produit des offres de credit n'aurait pas de sens.

CREATE TABLE agents (
    id           bigserial   PRIMARY KEY,
    -- Identifiant de connexion, compare en minuscules pour qu'une casse
    -- differente ne cree pas un second compte.
    identifiant  text        NOT NULL,
    -- Empreinte argon2id complete, parametres compris : le format porte sa
    -- propre configuration, ce qui permettra d'en durcir les couts sans
    -- invalider les empreintes deja calculees.
    mot_de_passe text        NOT NULL,

    nom          text        NOT NULL,
    agence       text        NOT NULL,
    -- Desactiver plutot que supprimer : les dossiers montes par l'agent
    -- doivent rester rattachables a leur auteur.
    actif        boolean     NOT NULL DEFAULT true,

    cree_le            timestamptz NOT NULL DEFAULT now(),
    derniere_connexion timestamptz
);

COMMENT ON TABLE agents IS
    'Comptes des agents de credit ; aucun mot de passe en clair';

-- L'unicite porte sur l'identifiant en minuscules.
CREATE UNIQUE INDEX agents_identifiant ON agents (lower(identifiant));

CREATE TABLE sessions (
    -- Empreinte SHA-256 du jeton remis au navigateur. Le jeton lui-meme
    -- n'existe que dans le cookie.
    empreinte    bytea       PRIMARY KEY,
    agent_id     bigint      NOT NULL REFERENCES agents (id) ON DELETE CASCADE,

    cree_le      timestamptz NOT NULL DEFAULT now(),
    expire_le    timestamptz NOT NULL,
    -- De quel poste la session a ete ouverte, pour qu'un agent puisse
    -- reconnaitre une connexion qui n'est pas la sienne.
    adresse      inet,
    agent_utilisateur text
);

COMMENT ON TABLE sessions IS
    'Sessions ouvertes ; seule l''empreinte du jeton est conservee';

-- Lister puis revoquer les sessions d'un agent.
CREATE INDEX sessions_agent ON sessions (agent_id);
-- La purge des sessions echues balaie par date.
CREATE INDEX sessions_expiration ON sessions (expire_le);
