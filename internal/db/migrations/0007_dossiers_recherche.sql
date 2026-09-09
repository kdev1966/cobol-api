-- Index pour la recherche par reference et pour la pagination par curseur.
--
-- La recherche : lower(reference) seul ne sert pas un LIKE 'prefixe%' hors
-- collation C, l'ordre des chaines n'y suivant pas celui des octets. Il faut
-- text_pattern_ops, qui compare octet a octet et rend donc le prefixe
-- utilisable. Mesure sur une agence de 200 000 dossiers, sans cet index :
-- 104 ms pour une reference, par balayage de toute l'agence.
--
-- La pagination : l'index (agence, cree_le DESC) ne porte pas l'identifiant,
-- qui departage les dossiers crees dans la meme seconde. Sans lui, le tri se
-- refait a chaque page. C'est aussi ce que la comparaison de couple
-- (cree_le, id) < (borne) exige pour tenir en un parcours d'index.
--
-- La pagination par curseur remplace OFFSET, dont le cout croit avec la
-- profondeur : sur ce meme volume, 0,6 ms a la premiere page mais 265 ms a la
-- 3800e, contre 17 ms par curseur quelle que soit la page.

CREATE INDEX dossiers_reference_prefixe
    ON dossiers (agence, lower(reference) text_pattern_ops);

CREATE INDEX dossiers_curseur
    ON dossiers (agence, cree_le DESC, id DESC);

-- Remplace par le precedent, qui le prefixe.
DROP INDEX IF EXISTS dossiers_agence;
