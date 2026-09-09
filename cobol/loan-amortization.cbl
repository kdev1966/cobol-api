       IDENTIFICATION DIVISION.
       PROGRAM-ID. LOAN-AMORTIZATION.

      *> Produit l'echeancier d'un pret, frais et assurance compris.
      *>
      *> L'arithmetique est en decimal a virgule fixe : les montants sont
      *> exacts au centime, ce qu'un flottant binaire ne garantit pas.
      *> Six invariants sont tenus par construction :
      *>   1. la somme des parts de capital egale le capital emprunte
      *>   2. la somme des echeances egale capital + interets
      *>   3. le solde apres la derniere echeance est nul
      *>   4. chaque ligne verifie echeance = capital + interets
      *>   5. chaque ligne verifie du = echeance + assurance
      *>   6. le total du egale la somme des echeances et des assurances
      *> La derniere echeance absorbe le residu d'arrondi, comme le veut
      *> la pratique bancaire.
      *>
      *> Un differe partiel peut preceder l'amortissement : pendant la
      *> franchise, l'emprunteur ne paie que les interets et l'assurance, le
      *> capital reste intact. L'amortissement se fait ensuite sur la duree
      *> restante. Les interets ne sont pas capitalises : un differe total le
      *> ferait, et demanderait de distinguer les interets payes de ceux
      *> ajoutes au capital.
      *>
      *> Methodes d'amortissement :
      *>   A  annuite constante : l'echeance ne bouge pas, la part de
      *>      capital croit a mesure que les interets diminuent
      *>   C  capital constant  : la part de capital ne bouge pas,
      *>      l'echeance decroit
      *>   I  in fine           : interets seuls, capital rembourse en
      *>      totalite a la derniere echeance
      *>
      *> Assiettes d'assurance :
      *>   N  aucune assurance
      *>   I  capital initial   : prime constante sur toute la duree
      *>   R  capital restant du : prime decroissante
      *>
      *> Les montants sont en millimes : le dinar tunisien se divise en mille,
      *> d'ou trois decimales sur tous les champs monetaires.
      *>
      *> Le taux effectif moyen de la categorie de concours est fourni en
      *> entree ; sa lecture, qui depend d'un bareme semestriel publie par
      *> arrete, releve de l'appelant. Ce programme garde la regle legale :
      *> majorer d'un cinquieme et statuer. Un TEM nul signifie qu'aucune
      *> verification n'est demandee.
      *>
      *> Entree : une ligne de 67 caracteres sur stdin
      *>            capital         9(11)V999  positions  1-14
      *>            taux annuel     9(2)V9(6)  positions 15-22
      *>            duree mois      9(4)       positions 23-26
      *>            frais dossier   9(9)V999   positions 27-38
      *>            frais garantie  9(9)V999   positions 39-50
      *>            taux assurance  9(2)V9(6)  positions 51-58
      *>            TEM categorie   9(2)V99    positions 59-62
      *>                            (zero = aucune verification)
      *>            differe mois    9(3)       positions 63-65
      *>                            (zero = aucun differe)
      *>            methode         X          position     66
      *>            assiette assur. X          position     67
      *>
      *> Sortie : enregistrements a largeur fixe, un par ligne.
      *>          "R" recapitulatif : echeances 9(4), premiere et derniere
      *>              echeance 9(11)V99, total interets, total assurance
      *>              9(13)V99, total frais 9(11)V99, total du et cout du
      *>              credit 9(13)V99, taeg 9(2)V9(4), taux d'usure
      *>              9(2)V9(4), conformite X, marge S9(2)V9(4) a signe
      *>              separe                                  -> 124 car.
      *>          "E" echeance      : numero 9(4), echeance, interets,
      *>              capital, assurance, du, solde 9(11)V99   -> 83 car.
      *>
      *>          JSON GENERATE n'est deliberement pas utilise : le paquet
      *>          GnuCOBOL des distributions est construit sans bibliotheque
      *>          JSON, et l'instruction compile alors sans rien produire, ni
      *>          erreur ni avertissement. La mise en forme revient a
      *>          l'appelant.
      *>
      *> Retour : 0 succes, 2 entree malformee, 3 parametres hors bornes,
      *>          4 methode ou assiette inconnue, 5 differe incompatible
      *>          avec la duree.
      *>
      *> Le TAEG est le taux actuariel annuel qui egalise la valeur actuelle
      *> de ce que l'emprunteur verse au montant qu'il percoit reellement,
      *> soit le capital diminue des frais. Il ne se deduit d'aucune formule
      *> des lors que des frais et une assurance entrent dans les flux : il
      *> est resolu par dichotomie, en arithmetique decimale exacte.

       ENVIRONMENT DIVISION.

       DATA DIVISION.
       WORKING-STORAGE SECTION.

       01 WS-ENTREE             PIC X(67) VALUE SPACES.
       01 WS-ENTREE-CHAMPS REDEFINES WS-ENTREE.
          05 WS-E-CHIFFRES      PIC X(65).
          05 WS-E-METHODE       PIC X.
          05 WS-E-ASSIETTE      PIC X.
       01 WS-E-DETAIL REDEFINES WS-ENTREE.
          05 WS-E-CAPITAL       PIC 9(11)V999.
          05 WS-E-TAUX          PIC 9(2)V9(6).
          05 WS-E-DUREE         PIC 9(4).
          05 WS-E-FRAIS-DOSSIER PIC 9(9)V999.
          05 WS-E-FRAIS-GARANTIE PIC 9(9)V999.
          05 WS-E-TAUX-ASSUR    PIC 9(2)V9(6).
          05 WS-E-TEM           PIC 9(2)V99.
          05 WS-E-DIFFERE       PIC 9(3).
          05 FILLER             PIC X(2).

       78 METHODE-ANNUITE       VALUE "A".
       78 METHODE-CAPITAL       VALUE "C".
       78 METHODE-IN-FINE       VALUE "I".
       78 ASSIETTE-AUCUNE       VALUE "N".
       78 ASSIETTE-INITIAL      VALUE "I".
       78 ASSIETTE-RESTANT      VALUE "R".
       78 CONFORME-OUI          VALUE "O".
       78 CONFORME-NON          VALUE "N".
       78 CONFORME-SANS-OBJET   VALUE "-".

       01 WS-CONFORME           PIC X      VALUE "-".
       01 WS-MARGE              PIC S9(2)V99   VALUE 0.

       01 WS-TAUX-MENSUEL       PIC 9V9(18)    VALUE 0.
       01 WS-TAUX-ASSUR-MENSUEL PIC 9V9(18)    VALUE 0.
       01 WS-FACTEUR            PIC 9(9)V9(18) VALUE 0.
       01 WS-MENSUALITE         PIC 9(11)V999  VALUE 0.
      *> Part de capital fixe de la methode a capital constant.
       01 WS-AMORT-FIXE         PIC 9(11)V999  VALUE 0.
      *> Nombre d'echeances effectivement amortissantes, la franchise deduite.
       01 WS-DUREE-AMORT        PIC 9(4)       VALUE 0.
       01 WS-FRAIS              PIC 9(11)V999  VALUE 0.
       01 WS-VERSE              PIC 9(11)V999  VALUE 0.

       01 WS-I                  PIC 9(4)       VALUE 0.
       01 WS-SOLDE              PIC S9(11)V999 VALUE 0.
       01 WS-INTERET            PIC 9(11)V999  VALUE 0.
       01 WS-PART-CAPITAL       PIC 9(11)V999  VALUE 0.
       01 WS-ECHEANCE           PIC 9(11)V999  VALUE 0.
       01 WS-ASSURANCE          PIC 9(11)V999  VALUE 0.
       01 WS-DU                 PIC 9(11)V999  VALUE 0.

       01 WS-CUM-INTERET        PIC 9(13)V999  VALUE 0.
       01 WS-CUM-ASSURANCE      PIC 9(13)V999  VALUE 0.
       01 WS-CUM-DU             PIC 9(13)V999  VALUE 0.
       01 WS-COUT-CREDIT        PIC 9(13)V999  VALUE 0.

      *> Duree maximale acceptee, qui dimensionne la table des versements
      *> conservee pour la resolution du TAEG.
       78 DUREE-MAX             VALUE 600.
       78 ITERATIONS-TAEG       VALUE 40.

       01 WS-VERSEMENTS.
          05 WS-VERSEMENT       PIC 9(11)V999 OCCURS 600 TIMES.

       01 WS-J                  PIC 9(4)       VALUE 0.
       01 WS-ITER               PIC 9(3)       VALUE 0.
       01 WS-TAUX-BAS           PIC 9V9(18)    VALUE 0.
       01 WS-TAUX-HAUT          PIC 9V9(18)    VALUE 0.
       01 WS-TAUX-ESSAI         PIC 9V9(18)    VALUE 0.
       01 WS-ESCOMPTE           PIC 9V9(18)    VALUE 0.
       01 WS-VALEUR-ACTUELLE    PIC 9(13)V9(6) VALUE 0.
       01 WS-TEG                PIC 9(2)V99    VALUE 0.
       01 WS-SEUIL              PIC 9(2)V99    VALUE 0.

      *> Le deroulement sert deux fois : une passe muette pour totaliser,
      *> une passe emettrice. Un seul corps de boucle, donc une seule
      *> regle de calcul.
       01 WS-EMETTRE            PIC X          VALUE "N".
       01 WS-PREMIERE           PIC 9(11)V999  VALUE 0.
       01 WS-DERNIERE           PIC 9(11)V999  VALUE 0.

       01 WS-RECAP.
          05 FILLER             PIC X          VALUE "R".
          05 WS-R-ECHEANCES     PIC 9(4)       VALUE 0.
          05 WS-R-PREMIERE      PIC 9(11)V999  VALUE 0.
          05 WS-R-DERNIERE      PIC 9(11)V999  VALUE 0.
          05 WS-R-INTERETS      PIC 9(13)V999  VALUE 0.
          05 WS-R-ASSURANCE     PIC 9(13)V999  VALUE 0.
          05 WS-R-FRAIS         PIC 9(11)V999  VALUE 0.
          05 WS-R-TOTAL-DU      PIC 9(13)V999  VALUE 0.
          05 WS-R-COUT          PIC 9(13)V999  VALUE 0.
          05 WS-R-TEG           PIC 9(2)V99    VALUE 0.
          05 WS-R-TEM           PIC 9(2)V99    VALUE 0.
          05 WS-R-SEUIL         PIC 9(2)V99    VALUE 0.
          05 WS-R-CONFORME      PIC X          VALUE "-".
          05 WS-R-MARGE         PIC S9(2)V99 SIGN IS LEADING SEPARATE.

       01 WS-LIGNE.
          05 FILLER             PIC X          VALUE "E".
          05 WS-L-NUMERO        PIC 9(4)       VALUE 0.
          05 WS-L-ECHEANCE      PIC 9(11)V999  VALUE 0.
          05 WS-L-INTERETS      PIC 9(11)V999  VALUE 0.
          05 WS-L-CAPITAL       PIC 9(11)V999  VALUE 0.
          05 WS-L-ASSURANCE     PIC 9(11)V999  VALUE 0.
          05 WS-L-DU            PIC 9(11)V999  VALUE 0.
          05 WS-L-SOLDE         PIC 9(11)V999  VALUE 0.

       PROCEDURE DIVISION.

       MAIN-PROCEDURE.
           PERFORM LIRE-DEMANDE
           PERFORM PREPARER-CALCUL

           MOVE "N" TO WS-EMETTRE
           PERFORM DEROULER-ECHEANCIER
           PERFORM CALCULER-TEG
           PERFORM VERIFIER-TAUX-EXCESSIF
           PERFORM ECRIRE-RECAPITULATIF

           MOVE "O" TO WS-EMETTRE
           PERFORM DEROULER-ECHEANCIER

           STOP RUN.

       LIRE-DEMANDE.
           ACCEPT WS-ENTREE

           IF WS-E-CHIFFRES IS NOT NUMERIC
               DISPLAY "entree malformee : 55 chiffres puis deux lettres"
                   UPON SYSERR
               MOVE 2 TO RETURN-CODE
               STOP RUN
           END-IF

           IF WS-E-CAPITAL = 0 OR WS-E-DUREE = 0
               DISPLAY "capital et duree doivent etre non nuls"
                   UPON SYSERR
               MOVE 3 TO RETURN-CODE
               STOP RUN
           END-IF

           IF WS-E-DUREE > DUREE-MAX
               DISPLAY "duree superieure a " DUREE-MAX " mois"
                   UPON SYSERR
               MOVE 3 TO RETURN-CODE
               STOP RUN
           END-IF

      *> Il doit rester au moins une echeance pour amortir le capital.
           IF WS-E-DIFFERE >= WS-E-DUREE
               DISPLAY "le differe doit laisser au moins une echeance "
                   "amortissante" UPON SYSERR
               MOVE 5 TO RETURN-CODE
               STOP RUN
           END-IF

           COMPUTE WS-DUREE-AMORT = WS-E-DUREE - WS-E-DIFFERE

           IF WS-E-METHODE NOT = METHODE-ANNUITE
              AND WS-E-METHODE NOT = METHODE-CAPITAL
              AND WS-E-METHODE NOT = METHODE-IN-FINE
               DISPLAY "methode inconnue : " WS-E-METHODE UPON SYSERR
               MOVE 4 TO RETURN-CODE
               STOP RUN
           END-IF

           IF WS-E-ASSIETTE NOT = ASSIETTE-AUCUNE
              AND WS-E-ASSIETTE NOT = ASSIETTE-INITIAL
              AND WS-E-ASSIETTE NOT = ASSIETTE-RESTANT
               DISPLAY "assiette d'assurance inconnue : "
                   WS-E-ASSIETTE UPON SYSERR
               MOVE 4 TO RETURN-CODE
               STOP RUN
           END-IF

      *> Les frais sont verses au depart : ils diminuent ce que
      *> l'emprunteur percoit, sans reduire ce qu'il rembourse.
           COMPUTE WS-FRAIS =
               WS-E-FRAIS-DOSSIER + WS-E-FRAIS-GARANTIE

           IF WS-FRAIS >= WS-E-CAPITAL
               DISPLAY "les frais absorbent le capital" UPON SYSERR
               MOVE 3 TO RETURN-CODE
               STOP RUN
           END-IF

           COMPUTE WS-VERSE = WS-E-CAPITAL - WS-FRAIS.

      *> Taux periodiques proportionnels : taux nominal annuel divise par
      *> douze. C'est la convention du taux nominal, distincte du taux
      *> actuariel equivalent.
       PREPARER-CALCUL.
           COMPUTE WS-TAUX-MENSUEL = WS-E-TAUX / 100 / 12
           COMPUTE WS-TAUX-ASSUR-MENSUEL = WS-E-TAUX-ASSUR / 100 / 12

           EVALUATE WS-E-METHODE
               WHEN METHODE-ANNUITE
      *> La mensualite se calcule sur les seules echeances amortissantes :
      *> le capital est intact a la sortie de la franchise.
                   IF WS-TAUX-MENSUEL = 0
      *> Sans interets la formule diviserait par zero.
                       COMPUTE WS-MENSUALITE ROUNDED =
                           WS-E-CAPITAL / WS-DUREE-AMORT
                   ELSE
                       COMPUTE WS-FACTEUR =
                           (1 + WS-TAUX-MENSUEL) ** WS-DUREE-AMORT
                       COMPUTE WS-MENSUALITE ROUNDED =
                           WS-E-CAPITAL * WS-TAUX-MENSUEL
                           / (1 - (1 / WS-FACTEUR))
                   END-IF

               WHEN METHODE-CAPITAL
                   COMPUTE WS-AMORT-FIXE ROUNDED =
                       WS-E-CAPITAL / WS-DUREE-AMORT

               WHEN METHODE-IN-FINE
      *> Aucun capital n'est rembourse avant la derniere echeance.
                   MOVE 0 TO WS-AMORT-FIXE
           END-EVALUATE.

       DEROULER-ECHEANCIER.
           MOVE WS-E-CAPITAL TO WS-SOLDE
           MOVE 0 TO WS-CUM-INTERET
           MOVE 0 TO WS-CUM-ASSURANCE
           MOVE 0 TO WS-CUM-DU

           PERFORM VARYING WS-I FROM 1 BY 1
               UNTIL WS-I > WS-E-DUREE

               COMPUTE WS-INTERET ROUNDED =
                   WS-SOLDE * WS-TAUX-MENSUEL

      *> La prime se calcule sur le solde avant remboursement.
               EVALUATE WS-E-ASSIETTE
                   WHEN ASSIETTE-AUCUNE
                       MOVE 0 TO WS-ASSURANCE
                   WHEN ASSIETTE-INITIAL
                       COMPUTE WS-ASSURANCE ROUNDED =
                           WS-E-CAPITAL * WS-TAUX-ASSUR-MENSUEL
                   WHEN ASSIETTE-RESTANT
                       COMPUTE WS-ASSURANCE ROUNDED =
                           WS-SOLDE * WS-TAUX-ASSUR-MENSUEL
               END-EVALUATE

               IF WS-I <= WS-E-DIFFERE
      *> Pendant la franchise, seuls les interets et l'assurance sont
      *> dus : le capital reste intact.
                   MOVE 0 TO WS-PART-CAPITAL
               ELSE
               IF WS-I = WS-E-DUREE
      *> Quelle que soit la methode, la derniere echeance solde le
      *> capital restant : c'est elle qui absorbe le residu accumule
      *> par les arrondis mensuels.
                   MOVE WS-SOLDE TO WS-PART-CAPITAL
               ELSE
                   EVALUATE WS-E-METHODE
                       WHEN METHODE-ANNUITE
                           COMPUTE WS-PART-CAPITAL =
                               WS-MENSUALITE - WS-INTERET
                       WHEN METHODE-CAPITAL
                           MOVE WS-AMORT-FIXE TO WS-PART-CAPITAL
                       WHEN METHODE-IN-FINE
                           MOVE 0 TO WS-PART-CAPITAL
                   END-EVALUATE
               END-IF
               END-IF

               COMPUTE WS-ECHEANCE = WS-PART-CAPITAL + WS-INTERET
               COMPUTE WS-DU = WS-ECHEANCE + WS-ASSURANCE

               SUBTRACT WS-PART-CAPITAL FROM WS-SOLDE
               ADD WS-INTERET    TO WS-CUM-INTERET
               ADD WS-ASSURANCE  TO WS-CUM-ASSURANCE
               ADD WS-DU         TO WS-CUM-DU

               MOVE WS-DU TO WS-VERSEMENT(WS-I)

               IF WS-I = 1
                   MOVE WS-DU TO WS-PREMIERE
               END-IF
               IF WS-I = WS-E-DUREE
                   MOVE WS-DU TO WS-DERNIERE
               END-IF

               IF WS-EMETTRE = "O"
                   PERFORM ECRIRE-LIGNE
               END-IF

           END-PERFORM

           COMPUTE WS-COUT-CREDIT =
               WS-CUM-INTERET + WS-CUM-ASSURANCE + WS-FRAIS.

      *> Dichotomie sur le taux periodique : la valeur actuelle des
      *> versements decroit quand le taux monte, l'encadrement se resserre
      *> donc de moitie a chaque tour.
      *>
      *> Les versements etant construits a partir du taux nominal, le taux
      *> recherche en est proche ; 0,25 par mois le majore largement, meme
      *> avec des frais et une assurance. Quarante tours ramenent alors
      *> l'incertitude a 2,3e-13, bien au-dela des quatre decimales rendues.
       CALCULER-TEG.
           MOVE 0 TO WS-TAUX-BAS
           MOVE 0.25 TO WS-TAUX-HAUT

           PERFORM VARYING WS-ITER FROM 1 BY 1
               UNTIL WS-ITER > ITERATIONS-TAEG
               COMPUTE WS-TAUX-ESSAI =
                   (WS-TAUX-BAS + WS-TAUX-HAUT) / 2
               PERFORM VALEUR-ACTUELLE-DES-VERSEMENTS
      *> L'emprunteur ne percoit que le capital diminue des frais.
               IF WS-VALEUR-ACTUELLE > WS-VERSE
                   MOVE WS-TAUX-ESSAI TO WS-TAUX-BAS
               ELSE
                   MOVE WS-TAUX-ESSAI TO WS-TAUX-HAUT
               END-IF
           END-PERFORM

      *> Le decret n° 2000-462 impose un taux annuel proportionnel au taux de
      *> periode : le taux mensuel resolu est multiplie par douze, et non
      *> capitalise. Le resultat s'exprime avec deux decimales.
           COMPUTE WS-TEG ROUNDED = WS-TAUX-ESSAI * 12 * 100.

       VALEUR-ACTUELLE-DES-VERSEMENTS.
           MOVE 0 TO WS-VALEUR-ACTUELLE
           MOVE 1 TO WS-ESCOMPTE

           PERFORM VARYING WS-J FROM 1 BY 1 UNTIL WS-J > WS-E-DUREE
               COMPUTE WS-ESCOMPTE = WS-ESCOMPTE / (1 + WS-TAUX-ESSAI)
               COMPUTE WS-VALEUR-ACTUELLE = WS-VALEUR-ACTUELLE
                   + WS-VERSEMENT(WS-J) * WS-ESCOMPTE
           END-PERFORM.

      *> Loi n° 99-64 du 15 juillet 1999 : est excessif tout pret dont le taux
      *> effectif global excede de plus du cinquieme le taux effectif moyen
      *> pratique au semestre precedent pour la meme categorie de concours.
      *> Le seuil est donc le TEM majore de vingt pour cent, arrondi a deux
      *> decimales comme les taux publies par arrete.
      *>
      *> L'appelant fournit le TEM, qui est une donnee trimestrielle ; la
      *> regle de majoration et le verdict restent ici. Un TEM nul signifie
      *> qu'aucune verification n'est demandee.
       VERIFIER-TAUX-EXCESSIF.
           IF WS-E-TEM = 0
               MOVE CONFORME-SANS-OBJET TO WS-CONFORME
               MOVE 0 TO WS-SEUIL
               MOVE 0 TO WS-MARGE
               EXIT PARAGRAPH
           END-IF

           COMPUTE WS-SEUIL ROUNDED = WS-E-TEM * 12 / 10

      *> « Excede de plus du cinquieme » : un TEG egal au seuil reste licite.
           COMPUTE WS-MARGE = WS-SEUIL - WS-TEG
           IF WS-TEG > WS-SEUIL
               MOVE CONFORME-NON TO WS-CONFORME
           ELSE
               MOVE CONFORME-OUI TO WS-CONFORME
           END-IF.

       ECRIRE-RECAPITULATIF.
           MOVE WS-E-DUREE       TO WS-R-ECHEANCES
           MOVE WS-PREMIERE      TO WS-R-PREMIERE
           MOVE WS-DERNIERE      TO WS-R-DERNIERE
           MOVE WS-CUM-INTERET   TO WS-R-INTERETS
           MOVE WS-CUM-ASSURANCE TO WS-R-ASSURANCE
           MOVE WS-FRAIS         TO WS-R-FRAIS
           MOVE WS-CUM-DU        TO WS-R-TOTAL-DU
           MOVE WS-COUT-CREDIT   TO WS-R-COUT
           MOVE WS-TEG           TO WS-R-TEG
           MOVE WS-E-TEM         TO WS-R-TEM
           MOVE WS-SEUIL         TO WS-R-SEUIL
           MOVE WS-CONFORME      TO WS-R-CONFORME
           MOVE WS-MARGE         TO WS-R-MARGE

           DISPLAY WS-RECAP.

       ECRIRE-LIGNE.
           MOVE WS-I            TO WS-L-NUMERO
           MOVE WS-ECHEANCE     TO WS-L-ECHEANCE
           MOVE WS-INTERET      TO WS-L-INTERETS
           MOVE WS-PART-CAPITAL TO WS-L-CAPITAL
           MOVE WS-ASSURANCE    TO WS-L-ASSURANCE
           MOVE WS-DU           TO WS-L-DU
           MOVE WS-SOLDE        TO WS-L-SOLDE

           DISPLAY WS-LIGNE.
