
#define PY_SSIZE_T_CLEAN
#include <Python.h>
#include <structmember.h>

#include <Havoc/PythonApi/PythonApi.h>
#include <Havoc/PythonApi/UI/PyTreeClass.hpp>


PyMemberDef PyTreeClass_members[] = {

        { "title",       T_STRING, offsetof( PyTreeClass, title ),    0, "title" },

        { NULL },
};

PyMethodDef PyTreeClass_methods[] = {

        { "setBottomTab",               ( PyCFunction ) TreeClass_setBottomTab,               METH_VARARGS, "Set widget as Bottom Tab" },
        { "setSmallTab",               ( PyCFunction ) TreeClass_setSmallTab,               METH_VARARGS, "Set widget as Small Tab" },
        { "addRow",               ( PyCFunction ) TreeClass_addRow,               METH_VARARGS, "add a row to the tree" },
        { "setPanel",               ( PyCFunction ) TreeClass_setPanel,               METH_VARARGS, "Set the data inside of the panel" },
        { "setItem",               ( PyCFunction ) TreeClass_setItem,               METH_VARARGS, "set an item in the tree" },

        { NULL },
};

PyTypeObject PyTreeClass_Type = {
        PyVarObject_HEAD_INIT( &PyType_Type, 0 )

        "havocui.tree",                              /* tp_name */
        sizeof( PyTreeClass ),                     /* tp_basicsize */
        0,                                          /* tp_itemsize */
        ( destructor ) TreeClass_dealloc,          /* tp_dealloc */
        0,                                          /* tp_print */
        0,                                          /* tp_getattr */
        0,                                          /* tp_setattr */
        0,                                          /* tp_reserved */
        0,                                          /* tp_repr */
        0,                                          /* tp_as_number */
        0,                                          /* tp_as_sequence */
        0,                                          /* tp_as_mapping */
        0,                                          /* tp_hash */
        0,                                          /* tp_call */
        0,                                          /* tp_str */
        0,                                          /* tp_getattro */
        0,                                          /* tp_setattro */
        0,                                          /* tp_as_buffer */
        Py_TPFLAGS_DEFAULT | Py_TPFLAGS_BASETYPE,   /* tp_flags */
        "Tree Widget Havoc Object",                      /* tp_doc */
        0,                                          /* tp_traverse */
        0,                                          /* tp_clear */
        0,                                          /* tp_richcompare */
        0,                                          /* tp_weaklistoffset */
        0,                                          /* tp_iter */
        0,                                          /* tp_iternext */
        PyTreeClass_methods,                       /* tp_methods */
        PyTreeClass_members,                       /* tp_members */
        0,                                          /* tp_getset */
        0,                                          /* tp_base */
        0,                                          /* tp_dict */
        0,                                          /* tp_descr_get */
        0,                                          /* tp_descr_set */
        0,                                          /* tp_dictoffset */
        ( initproc ) TreeClass_init,               /* tp_init */
        0,                                          /* tp_alloc */
        TreeClass_new,                             /* tp_new */
};

#define AllocMov( des, src, size )                          \
    if ( size > 0 )                                         \
    {                                                       \
        des = ( char* ) malloc( ( size + 1 ) * sizeof( char ) ); \
        memset( des, 0, size + 1 );                         \
        std::strncpy( des, src, size );                     \
    }

void TreeClass_dealloc( PPyTreeClass self )
{
    if (self) {
        /* title is a malloc'd char buffer (AllocMov), not a PyObject:
         * Py_DECREF on it corrupts the heap */
        if (self->title)
            free( self->title );
        /* the window may have been reparented into another widget: Qt owns
         * and deletes it then, deleting it here too would be a double free */
        if (self->TreeWindow && self->TreeWindow->window && self->TreeWindow->window->parent() == nullptr) {
            /* drop the destroyed handler first: its Py_DECREF(self) would
             * re-enter this dealloc mid-way through */
            self->TreeWindow->window->disconnect( self->TreeWindow->window, &QObject::destroyed, nullptr, nullptr );
            delete self->TreeWindow->window;
        }
        if (self->TreeWindow)
            free(self->TreeWindow);
        Py_TYPE( self )->tp_free( ( PyObject* ) self );
    }
}

PyObject* TreeClass_new( PyTypeObject *type, PyObject *args, PyObject *kwds )
{
    PPyTreeClass self;

    self = ( PPyTreeClass ) PyType_Type.tp_alloc( type, 0 );
    if (self == NULL)
        return NULL;
    self->title = NULL;
    self->TreeWindow = NULL;
    self->TreeWindow = (PPyTreeQWindow)malloc(sizeof(PyTreeQWindow));
    if (self->TreeWindow == NULL)
        return NULL;
    self->TreeWindow->window = NULL;
    self->TreeWindow->layout = NULL;
    self->TreeWindow->scroll= NULL;
    self->TreeWindow->root = NULL;
    self->TreeWindow->panel = NULL;
    self->TreeWindow->root_layout = NULL;
    return ( PyObject* ) self;
}

int TreeClass_init( PPyTreeClass self, PyObject *args, PyObject *kwds )
{
    char*       title          = NULL;
    PyObject* class_callback    = nullptr;
    PyObject* has_panel         = nullptr;
    const char* kwdlist[]        = { "title", "callback", "panel", NULL };

    if ( ! PyArg_ParseTupleAndKeywords( args, kwds, "sO|O", const_cast<char**>(kwdlist), &title, &class_callback, &has_panel ) )
        return -1;
    if ( !PyCallable_Check(class_callback) )
    {
        PyErr_SetString(PyExc_TypeError, "parameter must be callable");
        return -1;
    }

    AllocMov( self->title, title, strlen(title) );

    self->TreeWindow->window = new QWidget();
    self->TreeWindow->window->setWindowTitle(title);

    self->TreeWindow->root = new QWidget();
    self->TreeWindow->layout = new QHBoxLayout(self->TreeWindow->root);

    self->TreeWindow->scroll = new QScrollArea(self->TreeWindow->window);
    self->TreeWindow->scroll->setWidgetResizable(true);
    self->TreeWindow->scroll->setWidget(self->TreeWindow->root);

    self->TreeWindow->root_layout = new QVBoxLayout(self->TreeWindow->window);
    self->TreeWindow->root_layout->addWidget(self->TreeWindow->scroll);

    self->TreeWindow->item_model = new QStandardItemModel();
    self->TreeWindow->item_model->setColumnCount(1);

    self->TreeWindow->root_item = new QStandardItem(title);
    self->TreeWindow->tree_view = new QTreeView();
    self->TreeWindow->tree_view->setEditTriggers(QAbstractItemView::NoEditTriggers);

    self->TreeWindow->tree_view->setModel(self->TreeWindow->item_model);
    self->TreeWindow->item_model->invisibleRootItem()->appendRow(self->TreeWindow->root_item);

    if (has_panel && PyBool_Check(has_panel) && has_panel == Py_True) {
        self->TreeWindow->splitter = new QSplitter();
        self->TreeWindow->panel = new QTextBrowser();
        //self->TreeWindow->panel->setOpenLinks(false);
        self->TreeWindow->panel->setOpenExternalLinks(true);
        //self->TreeWindow->panel->setReadOnly(true);
        self->TreeWindow->splitter->addWidget(self->TreeWindow->tree_view);
        self->TreeWindow->splitter->addWidget(self->TreeWindow->panel);
        self->TreeWindow->layout->addWidget(self->TreeWindow->splitter);
    } else {
        self->TreeWindow->layout->addWidget(self->TreeWindow->tree_view);
    }

    /* borrowed reference from the args tuple: keep it alive for the view
     * lifetime and call it under the GIL */
    Py_INCREF( class_callback );
    /* the lambda captures self raw: hold an owned reference so the callback
     * can't fire after the Python object is deallocated. it is released (along
     * with the callback reference) when the Qt window is destroyed. */
    Py_INCREF( ( PyObject* ) self );
    QObject::connect(self->TreeWindow->tree_view->selectionModel(), &QItemSelectionModel::selectionChanged, self->TreeWindow->window, [self, class_callback](const QItemSelection &selected, const QItemSelection &deselected) {
        auto GilState = PyGILState_Ensure();

        for (const QModelIndex &index : selected.indexes()) {
            QStandardItem *selectedItem = self->TreeWindow->item_model->itemFromIndex(index);
            if (selectedItem) {
                const char *str = selectedItem->text().toUtf8().constData();
                PyObject* pystr = PyUnicode_DecodeFSDefault(str);
                PyObject* pResult = pystr ? PyObject_CallFunctionObjArgs(class_callback, pystr, nullptr) : nullptr;
                if ( pResult == nullptr && PyErr_Occurred() )
                {
                    PyErr_PrintEx( 0 );
                    PyErr_Clear();
                }
                Py_XDECREF( pResult );
                Py_XDECREF( pystr );
            }
        }

        PyGILState_Release( GilState );
    });
    QObject::connect(self->TreeWindow->window, &QObject::destroyed, [self, class_callback](QObject*) {
        auto GilState = PyGILState_Ensure();

        Py_XDECREF( class_callback );
        /* the window is gone: Qt owns the whole object tree, so every member
         * of the struct dangles — null them all so dealloc and later calls
         * can't touch freed Qt objects */
        self->TreeWindow->window      = nullptr;
        self->TreeWindow->layout      = nullptr;
        self->TreeWindow->scroll      = nullptr;
        self->TreeWindow->root        = nullptr;
        self->TreeWindow->root_layout = nullptr;
        self->TreeWindow->item_model  = nullptr;
        self->TreeWindow->root_item   = nullptr;
        self->TreeWindow->tree_view   = nullptr;
        self->TreeWindow->panel       = nullptr;
        self->TreeWindow->splitter    = nullptr;
        Py_DECREF( ( PyObject* ) self );

        PyGILState_Release( GilState );
    });

    return 0;
}

// Methods

PyObject* TreeClass_setBottomTab( PPyTreeClass self, PyObject *args )
{
    if ( ! HavocX::HavocUserInterface->NewBottomTab( self->TreeWindow->window, self->title) )
    {
        PyErr_SetString( PyExc_RuntimeError, "no teamserver session is active, cannot add tab" );
        return NULL;
    }

    Py_RETURN_NONE;
}

PyObject* TreeClass_setSmallTab( PPyTreeClass self, PyObject *args )
{
    if ( ! HavocX::HavocUserInterface->NewSmallTab( self->TreeWindow->window, self->title) )
    {
        PyErr_SetString( PyExc_RuntimeError, "no teamserver session is active, cannot add tab" );
        return NULL;
    }

    Py_RETURN_NONE;
}

PyObject* TreeClass_addRow( PPyTreeClass self, PyObject *args )
{
    const char *title = nullptr;
    Py_ssize_t tuple_size = PyTuple_Size(args);

    if (tuple_size < 1) {
        PyErr_SetString(PyExc_TypeError, "addRow requires at least a title string");
        return NULL;
    }

    title = (const char *)PyUnicode_AsUTF8(PyTuple_GetItem(args, 0));
    if (title == NULL) {
        PyErr_SetString(PyExc_TypeError, "First parameter must be a string");
        return NULL;
    }

    QStandardItem* child = new QStandardItem(title);
    QList<QStandardItem*> child_data;

    for (Py_ssize_t i = 1; i < tuple_size; i++) {
        const char* child_str = (const char *)PyUnicode_AsUTF8(PyTuple_GetItem(args, i));
        if (child_str == NULL) {
            PyErr_SetString(PyExc_TypeError, "Row values must be strings");
            delete child;
            qDeleteAll(child_data);
            return NULL;
        }
        child_data.append(new QStandardItem(child_str));
    }
    child->appendColumn(child_data);
    self->TreeWindow->root_item->appendRow(child);

    Py_RETURN_NONE;
}

PyObject* TreeClass_setItem( PPyTreeClass self, PyObject *args )
{
    int x, y;
    char *str;
    if( !PyArg_ParseTuple( args, "iis", &x, &y, &str) )
    {
        Py_RETURN_NONE;
    }
    QStandardItem* element = new QStandardItem(str);

    self->TreeWindow->item_model->setItem(x, y, element);

    Py_RETURN_NONE;
}

PyObject* TreeClass_setPanel( PPyTreeClass self, PyObject *args )
{
    char *str;
    if( !PyArg_ParseTuple( args, "s", &str) )
    {
        Py_RETURN_NONE;
    }
    if (self->TreeWindow->panel) {
        self->TreeWindow->panel->clear();

        /* scripts pass intentional HTML markup here; render it as-is */
        self->TreeWindow->panel->setHtml( QString( str ) );
    } else {
        PyErr_SetString(PyExc_TypeError, "The tree panel was not activated on initialization");
        return NULL;
    }

    Py_RETURN_NONE;
}
