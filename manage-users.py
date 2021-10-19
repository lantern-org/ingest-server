import json, os, getpass, bcrypt, base64

fname = input("gimme your database json file [./database.json] ")
if fname == "":
    fname = "database.json"

# read file
db = {}
if os.path.exists(fname):
    db = {x['username']:x['password'] for x in json.load(open(fname,'r'))}

# edit file
query = input("what do you wanna do? [new/del] ")
if query == "new":
    un = input("username: ")
    if un == "":
        print("empty username")
        exit()
    if db.get(un):
        cont = input("user already exists -- do you want to update? [Y/no] ")
        if cont not in ['y','Y','']:
            exit()
    # TODO do further username validation
    pw1 = getpass.getpass("password: ")
    pw2 = getpass.getpass("retype password: ")
    if pw1 == "":
        print("empty password")
        exit()
    if pw1 != pw2:
        print("password mismatch -- try again")
        exit()
    if len(pw1) > 72:
        print("can't handle passwords of that length yet")
        exit()
    # hash password
    hashed = bcrypt.hashpw(pw1.encode('utf-8'), bcrypt.gensalt())
    db[un] = base64.b64encode(hashed).decode('ascii')
    print("saved user.")
elif query == "del":
    print()
else:
    print("invalid option " + query)

# write file
json.dump([{'username':x,'password':db[x]} for x in db], open(fname,'w'))    
print("wrote database.")
